package session_test

import (
	"context"
	"net"
	"testing"
	"time"

	"xata/services/gateway/session"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestServeSQLSession_BidirectionalDataFlow(t *testing.T) {
	tracer := noop.NewTracerProvider().Tracer("test")
	clientConn, proxyClientConn := net.Pipe()
	proxyServerConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	sess := session.New(tracer, "test-branch", proxyClientConn, proxyServerConn)
	ctx := context.Background()

	errCh := make(chan error, 1)
	go func() {
		errCh <- sess.ServeSQLSession(ctx)
	}()

	// Test client -> server
	trySend(t, clientConn, serverConn, "SELECT 1;")

	// Test server -> client
	trySend(t, serverConn, clientConn, "result data")

	// Close connections to end session
	clientConn.Close()
	serverConn.Close()

	// Wait for session to complete
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("session did not complete in time")
	}
}

func TestServeSQLSession_ContextCancelledBeforeStart(t *testing.T) {
	tracer := noop.NewTracerProvider().Tracer("test")
	clientConn, proxyClientConn := net.Pipe()
	proxyServerConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	sess := session.New(tracer, "test-branch", proxyClientConn, proxyServerConn)

	// Cancel context before starting session
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := sess.ServeSQLSession(ctx)
	require.NoError(t, err) // Context cancellation should not be an error
}

func TestServeSQLSession_ContextCancelledDuringSession(t *testing.T) {
	tracer := noop.NewTracerProvider().Tracer("test")
	clientConn, proxyClientConn := net.Pipe()
	proxyServerConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	sess := session.New(tracer, "test-branch", proxyClientConn, proxyServerConn)
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- sess.ServeSQLSession(ctx)
	}()

	// First, establish communication works
	trySend(t, clientConn, serverConn, "SELECT 1;")

	// Now cancel the context
	cancel()

	// Wait for session to complete due to cancellation
	select {
	case err := <-errCh:
		require.NoError(t, err) // Context cancellation should not be an error
	case <-time.After(100 * time.Millisecond):
		t.Fatal("session did not complete after context cancellation")
	}

	// Verify that communication is no longer possible after cancellation
	// This should fail because the session should have closed the connections
	_, err := clientConn.Write([]byte("after cancel"))
	require.Error(t, err, "expected write to fail after session cancellation")
}

func TestServeSQLSession_ConnectionErrorPropagation(t *testing.T) {
	tracer := noop.NewTracerProvider().Tracer("test")
	clientConn, proxyClientConn := net.Pipe()
	proxyServerConn, serverConn := net.Pipe()
	defer serverConn.Close()

	sess := session.New(tracer, "test-branch", proxyClientConn, proxyServerConn)

	ctx := context.Background()

	errCh := make(chan error, 1)
	go func() {
		errCh <- sess.ServeSQLSession(ctx)
	}()

	// Close one connection early to trigger error handling
	time.Sleep(10 * time.Millisecond)
	clientConn.Close()

	// Wait for session to complete
	select {
	case err := <-errCh:
		require.NoError(t, err) // Connection close should be handled gracefully
	case <-time.After(100 * time.Millisecond):
		t.Fatal("session did not complete in time")
	}
}

func trySend(t *testing.T, from net.Conn, to net.Conn, data string) {
	_, err := from.Write([]byte(data))
	require.NoError(t, err)
	buffer := make([]byte, len(data))
	n, err := to.Read(buffer)
	require.NoError(t, err)
	require.Equal(t, data, string(buffer[:n]))
}
