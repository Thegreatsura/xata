package gateway

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"xata/services/gateway/metrics"
	"xata/services/gateway/session"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/metric/noop"
)

// stubSession blocks in ServeSQLSession until release is closed or the
// session context is cancelled.
type stubSession struct {
	release chan struct{}
}

func (s *stubSession) BranchID() string { return "test-branch" }

func (s *stubSession) ServeSQLSession(ctx context.Context) error {
	select {
	case <-s.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// stubInitiator hands each accepted session to the test via the sessions
// channel.
type stubInitiator struct {
	sessions chan *stubSession
}

func (si *stubInitiator) InitSession(ctx context.Context, sessionID string, conn net.Conn) (session.Session, error) {
	s := &stubSession{release: make(chan struct{})}
	si.sessions <- s
	return s, nil
}

type branchError struct {
	err error
}

func (e *branchError) Error() string    { return e.err.Error() }
func (e *branchError) Unwrap() error    { return e.err }
func (e *branchError) BranchID() string { return "test-branch" }

type errorInitiator struct{}

func (errorInitiator) InitSession(context.Context, string, net.Conn) (session.Session, error) {
	return nil, &branchError{err: errors.New("session failed")}
}

func startTestServer(t *testing.T, drainingTime time.Duration) (addr string, sessions chan *stubSession, runErr chan error, cancel context.CancelFunc) {
	return startTestServerWithSignal(t, drainingTime, nil)
}

// startTestServerWithSignal starts a test server. The optional signal
// function derives the ServerConfig.ShutdownSignal context from the run
// context; when nil, no shutdown signal is configured.
func startTestServerWithSignal(t *testing.T, drainingTime time.Duration, signal func(runCtx context.Context) context.Context) (addr string, sessions chan *stubSession, runErr chan error, cancel context.CancelFunc) {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr = l.Addr().String()

	m, err := metrics.New(noop.NewMeterProvider().Meter("test"))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	cfg := ServerConfig{Listen: addr, DrainingTime: drainingTime}
	if signal != nil {
		cfg.ShutdownSignal = signal(ctx)
	}

	si := &stubInitiator{sessions: make(chan *stubSession, 1)}
	srv, ok := NewServer(si, cfg, m).(*server)
	require.True(t, ok)

	runErr = make(chan error, 1)
	go func() { runErr <- srv.runWithListener(ctx, l) }()

	return addr, si.sessions, runErr, cancel
}

// dial connects to the server; the listener is already bound when the server
// starts, so no retry is needed.
func dial(t *testing.T, addr string) net.Conn {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	return conn
}

func requireRunning(t *testing.T, runErr chan error, wait time.Duration) {
	t.Helper()
	select {
	case err := <-runErr:
		t.Fatalf("Run returned early: %v", err)
	case <-time.After(wait):
	}
}

func requireStopped(t *testing.T, runErr chan error, wait time.Duration) {
	t.Helper()
	select {
	case err := <-runErr:
		require.NoError(t, err)
	case <-time.After(wait):
		t.Fatal("Run did not return")
	}
}

func TestServerShutdownDrainsActiveConnections(t *testing.T) {
	addr, sessions, runErr, cancel := startTestServer(t, 30*time.Second)

	conn := dial(t, addr)
	defer conn.Close()
	sess := <-sessions

	cancel()

	// Run must block while the session is still active.
	requireRunning(t, runErr, 300*time.Millisecond)

	// New connections are refused while draining.
	require.Eventually(t, func() bool {
		c, err := net.Dial("tcp", addr)
		if err == nil {
			c.Close()
			return false
		}
		return true
	}, 5*time.Second, 10*time.Millisecond)

	// Once the session ends, Run returns without error.
	close(sess.release)
	requireStopped(t, runErr, 5*time.Second)
}

func TestServerShutdownDrainingTimeout(t *testing.T) {
	addr, sessions, runErr, cancel := startTestServer(t, 300*time.Millisecond)

	conn := dial(t, addr)
	defer conn.Close()
	<-sessions // session is never released

	start := time.Now()
	cancel()

	requireStopped(t, runErr, 5*time.Second)
	require.GreaterOrEqual(t, time.Since(start), 300*time.Millisecond)
}

func TestServerShutdownNoActiveConnections(t *testing.T) {
	addr, sessions, runErr, cancel := startTestServer(t, 30*time.Second)

	// Make sure the server is up before shutting it down, and end the
	// session so nothing is left to drain.
	conn := dial(t, addr)
	sess := <-sessions
	close(sess.release)
	require.NoError(t, conn.Close())

	cancel()

	// Without active connections Run must return promptly, well before the
	// draining timeout.
	requireStopped(t, runErr, 5*time.Second)
}

func TestServerShutdownSignalDrains(t *testing.T) {
	// The shutdown signal is the run context itself, as in the non-HTTP
	// production wiring: cancelling it is a graceful shutdown and drains.
	addr, sessions, runErr, cancel := startTestServerWithSignal(t, 30*time.Second,
		func(runCtx context.Context) context.Context { return runCtx })

	conn := dial(t, addr)
	defer conn.Close()
	sess := <-sessions

	cancel()

	requireRunning(t, runErr, 300*time.Millisecond)

	close(sess.release)
	requireStopped(t, runErr, 5*time.Second)
}

func TestServerSiblingFailureSkipsDraining(t *testing.T) {
	// The shutdown signal stays uncancelled while the run context is
	// cancelled, as happens when a sibling failure cancels the shared
	// errgroup context: the server must fail fast instead of draining.
	addr, sessions, runErr, cancel := startTestServerWithSignal(t, 30*time.Second,
		func(runCtx context.Context) context.Context { return context.Background() })

	conn := dial(t, addr)
	defer conn.Close()
	sess := <-sessions
	t.Cleanup(func() { close(sess.release) })

	cancel()

	select {
	case err := <-runErr:
		require.Error(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return despite sibling failure")
	}

	// Sessions must be released promptly instead of draining: the client
	// connection gets closed once the shutdown hook returns.
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, err := conn.Read(make([]byte, 1))
	require.Error(t, err)
	require.NotErrorIs(t, err, os.ErrDeadlineExceeded)
}

func TestStartSessionReturnsBranchIDFromError(t *testing.T) {
	client, serverConn := net.Pipe()
	t.Cleanup(func() {
		client.Close()
		serverConn.Close()
	})

	srv := &server{
		initiator: errorInitiator{},
		drainer:   newTimedWaitGroup(time.Second),
	}

	branchID, err := srv.startSession(context.Background(), "session-id", serverConn)
	require.EqualError(t, err, "session failed")
	require.Equal(t, "test-branch", branchID)
}

// TestServerRunCancelRace cancels the run context concurrently with server
// startup: the shutdown hook may run before runWithListener finishes setting
// up and must not race it (caught by the race detector).
func TestServerRunCancelRace(t *testing.T) {
	m, err := metrics.New(noop.NewMeterProvider().Meter("test"))
	require.NoError(t, err)

	for i := range 1000 {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		si := &stubInitiator{sessions: make(chan *stubSession, 1)}
		srv, ok := NewServer(si, ServerConfig{Listen: l.Addr().String()}, m).(*server)
		require.True(t, ok)

		delay := time.Duration(i%2000) * time.Nanosecond
		go func() {
			time.Sleep(delay)
			cancel()
		}()
		_ = srv.runWithListener(ctx, l)
		cancel()
	}
}
