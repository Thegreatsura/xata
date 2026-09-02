package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"

	"xata/services/gateway/metrics"

	"github.com/elastic/go-concert/ctxtool"
	"github.com/elastic/go-concert/unison"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/trace"
)

type Session interface {
	ServeSQLSession(ctx context.Context) error
	BranchID() string
}

type session struct {
	tracer       trace.Tracer
	branch       string
	inboundConn  net.Conn
	outboundConn net.Conn
	metrics      *metrics.GatewayMetrics

	// Bytes moved in each direction, reported on the session's closing log
	// line. Written by the copy goroutines, read after they have both
	// finished.
	bytesToBackend atomic.Int64
	bytesToClient  atomic.Int64

	// The kernel's view of the backend connection, which is only readable
	// while its socket is open. See captureBackendTCPInfo.
	tcpInfoOnce sync.Once
	tcpInfo     atomic.Pointer[backendTCPInfo]
}

const (
	directionBackendToFrontend = "postgres -> client"
	directionFrontendToBackend = "client -> postgres"
	sideFrontend               = "frontend"
)

type copyResult struct {
	direction        string
	err              error
	contextCancelled bool
}

type terminationDetails struct {
	reason    string
	side      string
	operation string
}

// New creates a session proxying between a client and a backend connection.
// gwMetrics may be nil, in which case no metrics are recorded.
func New(
	tracer trace.Tracer,
	branch string,
	inboundConn, outboundConn net.Conn,
	gwMetrics *metrics.GatewayMetrics,
) Session {
	return &session{
		tracer:       tracer,
		branch:       branch,
		inboundConn:  inboundConn,
		outboundConn: outboundConn,
		metrics:      gwMetrics,
	}
}

func (s *session) BranchID() string { return s.branch }

func (s *session) ServeSQLSession(ctx context.Context) error {
	// Set the context that will be used during the `close` call.
	// This is needed to avoid race conditions when the context is cancelled
	// before or while `ServeSQLSession` sets up the context, overwriting `ctx`
	// variable in that context.
	closeCtx := ctx
	ctx, cancel := ctxtool.WithFunc(ctx, func() { s.close(closeCtx) })
	defer cancel()

	logger := log.Ctx(ctx).With().Str("branchID", s.branch).Logger()
	ctx = logger.WithContext(ctx)

	logger.Info().Msg("Start serving SQL session")
	// Reported as a closure so the byte counts are read after both copy
	// goroutines have finished, rather than captured when the defer is set up.
	defer func() {
		// close runs asynchronously, so it may or may not have captured this
		// already; whichever gets there first does it while the socket is open.
		s.captureBackendTCPInfo()

		event := logger.Info().
			Int64("bytes_to_backend", s.bytesToBackend.Load()).
			Int64("bytes_to_client", s.bytesToClient.Load())
		if info := s.tcpInfo.Load(); info != nil {
			event = event.
				Uint64("backend_bytes_acked", info.BytesAcked).
				Uint64("backend_bytes_retrans", info.BytesRetrans).
				Uint32("backend_unacked", info.Unacked).
				Uint32("backend_notsent", info.NotsentBytes).
				Uint32("backend_total_retrans", info.TotalRetrans).
				Int("backend_recv_queue", info.RecvQueue)
		}
		event.Msg("End serving SQL session")
	}()
	if ctx.Err() != nil {
		logger.Info().
			Str("termination_reason", "context_cancelled").
			Str("termination_side", "gateway").
			Msg("SQL session terminated")
		return nil
	}

	results := make(chan copyResult, 2)
	tg := unison.TaskGroupWithCancel(ctx)
	tg.OnQuit = unison.StopAll
	tg.Go(func(ctx context.Context) error {
		n, err := copyLoop(s.inboundConn, s.outboundConn)
		s.bytesToClient.Store(n)
		s.metrics.RecordBytesForwarded(ctx, metrics.DirectionBackendToClient, n)
		results <- copyResult{
			direction:        directionBackendToFrontend,
			err:              err,
			contextCancelled: closeCtx.Err() != nil,
		}
		return nil
	})
	tg.Go(func(ctx context.Context) error {
		n, err := copyLoop(s.outboundConn, s.inboundConn)
		s.bytesToBackend.Store(n)
		s.metrics.RecordBytesForwarded(ctx, metrics.DirectionClientToBackend, n)
		results <- copyResult{
			direction:        directionFrontendToBackend,
			err:              err,
			contextCancelled: closeCtx.Err() != nil,
		}
		return nil
	})

	// io.Copy returns only when its source reaches EOF or a read/write fails,
	// and neither loop closes a socket on its own. Therefore, unless the parent
	// context was already cancelled, the first result is the event that ended
	// the session. Save it before cancel closes both sockets; the sibling error
	// is then a cleanup effect.
	first, received := awaitCopyResult(closeCtx, results)
	cancel()
	tg.Wait()
	if !received {
		logger.Info().
			Str("termination_reason", "context_cancelled").
			Str("termination_side", "gateway").
			Msg("SQL session terminated")
		return nil
	}

	details := classifyTermination(first.direction, first.err, first.contextCancelled)
	event := logger.Info().
		Str("direction", first.direction).
		Str("termination_reason", details.reason).
		Str("termination_side", details.side)
	if details.operation != "" {
		event = event.Str("socket_operation", details.operation)
	}
	if first.err != nil {
		event = event.Err(first.err)
	}
	event.Msg("SQL session terminated")
	return nil
}

// captureBackendTCPInfo snapshots what the kernel knows about the backend
// connection: how much of what we wrote was acknowledged, and how much is
// still outstanding. It is only readable while the socket is open, and the
// session is torn down from two directions - close, driven asynchronously by
// the context hook, and the final log in ServeSQLSession - so both call this
// and the once decides. close calls it before closing anything, which also
// means a concurrent caller cannot lose the race to the socket being closed.
func (s *session) captureBackendTCPInfo() {
	s.tcpInfoOnce.Do(func() {
		info, err := readTCPInfo(s.outboundConn)
		if err != nil || info == nil {
			return
		}
		s.tcpInfo.Store(info)
	})
}

func awaitCopyResult(ctx context.Context, results <-chan copyResult) (copyResult, bool) {
	select {
	case result := <-results:
		return result, true
	case <-ctx.Done():
		select {
		case result := <-results:
			return result, true
		default:
			return copyResult{}, false
		}
	}
}

func (s *session) close(ctx context.Context) {
	s.captureBackendTCPInfo()

	if s.inboundConn != nil {
		if err := s.inboundConn.Close(); err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("close inbound connection")
		}
	}
	if s.outboundConn != nil {
		if err := s.outboundConn.Close(); err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("close outbound connection")
		}
	}
}

// copyLoop copies until the source is exhausted, returning the number of bytes
// written. The count is returned on error too, so a partially forwarded stream
// is still accounted for.
func copyLoop(to io.Writer, from io.Reader) (int64, error) {
	n, err := io.Copy(to, from)
	if err != nil {
		return n, fmt.Errorf("copy loop: %+w [%T]", err, err)
	}
	return n, nil
}

func classifyTermination(direction string, err error, contextCancelled bool) terminationDetails {
	operation := socketOperation(err)
	if contextCancelled {
		return terminationDetails{
			reason:    "context_cancelled",
			side:      "gateway",
			operation: operation,
		}
	}

	side := terminationSide(direction, operation)
	if err == nil || errors.Is(err, io.EOF) {
		return terminationDetails{
			reason:    side + "_fin",
			side:      side,
			operation: operation,
		}
	}
	if side == sideFrontend && errors.Is(err, io.ErrUnexpectedEOF) {
		return terminationDetails{
			reason:    "frontend_tls_unexpected_eof",
			side:      side,
			operation: operation,
		}
	}
	if errors.Is(err, syscall.ECONNRESET) || strings.Contains(err.Error(), "connection reset by peer") {
		return terminationDetails{
			reason:    side + "_reset",
			side:      side,
			operation: operation,
		}
	}
	if isTimeout(err) {
		return terminationDetails{
			reason:    side + "_timeout",
			side:      side,
			operation: operation,
		}
	}
	if errors.Is(err, syscall.EPIPE) {
		return terminationDetails{
			reason:    side + "_broken_pipe",
			side:      side,
			operation: operation,
		}
	}
	if errors.Is(err, net.ErrClosed) || strings.Contains(err.Error(), "use of closed network connection") {
		return terminationDetails{
			reason:    "gateway_close",
			side:      "gateway",
			operation: operation,
		}
	}

	return terminationDetails{
		reason:    side + "_io_error",
		side:      side,
		operation: operation,
	}
}

func socketOperation(err error) string {
	operation := ""
	for err != nil {
		if opErr, ok := err.(*net.OpError); ok { //nolint:errorlint // Inspect each wrapper to find the innermost operation.
			operation = opErr.Op
		}
		err = errors.Unwrap(err)
	}
	return operation
}

func terminationSide(direction, operation string) string {
	if operation == "write" {
		if direction == directionBackendToFrontend {
			return sideFrontend
		}
		return "backend"
	}

	if direction == directionBackendToFrontend {
		return "backend"
	}
	return sideFrontend
}

func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
