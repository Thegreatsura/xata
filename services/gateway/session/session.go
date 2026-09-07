package session

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"syscall"

	"xata/services/gateway/metrics"

	"github.com/elastic/go-concert/ctxtool"
	"github.com/elastic/go-concert/unison"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sys/unix"
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
	observedAtClient  = "client"
	observedAtServer  = "server"
	observedAtGateway = "gateway"
)

type copyDirection uint8

const (
	directionServerToClient copyDirection = iota
	directionClientToServer
)

type copyResult struct {
	direction        copyDirection
	err              error
	contextCancelled bool
}

type terminationDetails struct {
	// observedAt is the connection boundary that exposed the first copy
	// result. It does not claim that endpoint initiated the termination.
	observedAt string
	errorType  string
	operation  string
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
		logSessionTermination(logger, terminationDetails{
			observedAt: observedAtGateway,
			errorType:  "context.Canceled",
		}, nil)
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
			direction:        directionServerToClient,
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
			direction:        directionClientToServer,
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
		logSessionTermination(logger, terminationDetails{
			observedAt: observedAtGateway,
			errorType:  "context.Canceled",
		}, nil)
		return nil
	}

	details := classifyTermination(first.direction, first.err, first.contextCancelled)
	logSessionTermination(logger, details, first.err)
	return nil
}

func logSessionTermination(logger zerolog.Logger, details terminationDetails, err error) {
	event := logger.Info().
		Str("network.transport", "tcp").
		Str("network.protocol.name", "postgresql").
		Str("gateway.session.end.observed_at", details.observedAt)
	if details.errorType != "" {
		event = event.Str("error.type", details.errorType)
	}
	if details.operation != "" {
		event = event.Str("gateway.session.end.operation", details.operation)
	}
	if err != nil {
		event = event.Err(err)
	}
	event.Msg("SQL session terminated")
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

func classifyTermination(direction copyDirection, err error, contextCancelled bool) terminationDetails {
	operation := socketOperation(err)
	if contextCancelled {
		return terminationDetails{
			observedAt: observedAtGateway,
			errorType:  "context.Canceled",
			operation:  operation,
		}
	}

	observedAt := terminationObservedAt(direction, operation)
	errnoType := errnoName(err)
	var recordHeaderError tls.RecordHeaderError
	errorType := "_OTHER"
	switch {
	case err == nil || errors.Is(err, io.EOF):
		errorType = ""
	case errors.Is(err, io.ErrUnexpectedEOF):
		errorType = "io.ErrUnexpectedEOF"
	case errors.Is(err, net.ErrClosed):
		observedAt = observedAtGateway
		errorType = "net.ErrClosed"
	case errnoType != "":
		errorType = errnoType
	case isTimeout(err):
		errorType = "timeout"
	case errors.As(err, &recordHeaderError):
		errorType = "crypto/tls.RecordHeaderError"
	}

	return terminationDetails{
		observedAt: observedAt,
		errorType:  errorType,
		operation:  operation,
	}
}

func errnoName(err error) string {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return ""
	}
	return unix.ErrnoName(errno)
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

func terminationObservedAt(direction copyDirection, operation string) string {
	if operation == "write" {
		if direction == directionServerToClient {
			return observedAtClient
		}
		return observedAtServer
	}

	if direction == directionServerToClient {
		return observedAtServer
	}
	return observedAtClient
}

func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
