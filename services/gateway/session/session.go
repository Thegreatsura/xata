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

// New creates a session proxying between a client and a backend connection.
// gwMetrics may be nil, in which case no metrics are recorded.
func New(tracer trace.Tracer, branch string, inboundConn, outboundConn net.Conn, gwMetrics *metrics.GatewayMetrics) Session {
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
				Uint32("backend_total_retrans", info.TotalRetrans)
		}
		event.Msg("End serving SQL session")
	}()

	tg := unison.TaskGroupWithCancel(ctx)
	tg.OnQuit = unison.StopAll
	tg.Go(func(ctx context.Context) error {
		defer cancel()
		logger := log.Ctx(ctx).With().Str("direction", "postgres -> client").Logger()
		ctx = logger.WithContext(ctx)

		n, err := copyLoop(ctx, s.branch, s.inboundConn, s.outboundConn)
		s.bytesToClient.Store(n)
		s.metrics.RecordBytesForwarded(ctx, metrics.DirectionBackendToClient, n)
		if err != nil {
			logger.Error().Err(err).Msg("Copy loop error")
		}
		return nil
	})
	tg.Go(func(ctx context.Context) error {
		defer cancel()
		logger := log.Ctx(ctx).With().Str("direction", "client -> postgres").Logger()
		ctx = logger.WithContext(ctx)

		n, err := copyLoop(ctx, s.branch, s.outboundConn, s.inboundConn)
		s.bytesToBackend.Store(n)
		s.metrics.RecordBytesForwarded(ctx, metrics.DirectionClientToBackend, n)
		if err != nil {
			logger.Error().Err(err).Msg("Copy loop error")
		}
		return nil
	})
	tg.Wait()
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
func copyLoop(ctx context.Context, branch string, to io.Writer, from io.Reader) (int64, error) {
	n, err := io.Copy(to, from)
	if err != nil {
		if errors.Is(err, io.EOF) || isClosedConnError(err) {
			log.Ctx(ctx).Info().Msgf("connection from branch [%s] has been closed", branch)
			return n, nil
		}

		if netOpError, ok := errors.AsType[*net.OpError](err); ok {
			if netOpError.Op == "read" {
				if wrappedErr := errors.Unwrap(err); wrappedErr != nil {
					log.Ctx(ctx).Info().Err(wrappedErr).Msgf("wrapped error: %T", wrappedErr)
				}
			}
		}

		// return err
		return n, fmt.Errorf("copy loop: %+w [%T]", err, err)
	}
	return n, nil
}

func isClosedConnError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, net.ErrClosed) {
		return true
	}

	// some error values are private e.g. poll.errNetClosed. These are normally
	// wrapped inside a net.OpError.
	// Unfortunately we need to test by string matching the error message.

	var opErr *net.OpError
	if !errors.As(err, &opErr) {
		return false
	}
	if err = opErr.Unwrap(); err == nil {
		return false
	}

	errmsg := err.Error()
	if strings.Contains(errmsg, "use of closed network connection") {
		return true
	}
	if strings.Contains(errmsg, "reset by peer") {
		return true
	}

	return false
}
