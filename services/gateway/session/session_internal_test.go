package session

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"math/big"
	"net"
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestClassifyTermination(t *testing.T) {
	tests := map[string]struct {
		direction        string
		err              error
		contextCancelled bool
		want             terminationDetails
	}{
		"frontend FIN": {
			direction: directionFrontendToBackend,
			want: terminationDetails{
				reason: "frontend_fin",
				side:   "frontend",
			},
		},
		"backend FIN": {
			direction: directionBackendToFrontend,
			want: terminationDetails{
				reason: "backend_fin",
				side:   "backend",
			},
		},
		"frontend read reset": {
			direction: directionFrontendToBackend,
			err: &net.OpError{
				Op:  "read",
				Err: syscall.ECONNRESET,
			},
			want: terminationDetails{
				reason:    "frontend_reset",
				side:      "frontend",
				operation: "read",
			},
		},
		"frontend write reset": {
			direction: directionBackendToFrontend,
			err: &net.OpError{
				Op: "readfrom",
				Err: &net.OpError{
					Op:  "write",
					Err: syscall.ECONNRESET,
				},
			},
			want: terminationDetails{
				reason:    "frontend_reset",
				side:      "frontend",
				operation: "write",
			},
		},
		"frontend TLS unexpected EOF": {
			direction: directionFrontendToBackend,
			err: &net.OpError{
				Op:  "readfrom",
				Err: io.ErrUnexpectedEOF,
			},
			want: terminationDetails{
				reason:    "frontend_tls_unexpected_eof",
				side:      "frontend",
				operation: "readfrom",
			},
		},
		"backend timeout": {
			direction: directionBackendToFrontend,
			err: &net.OpError{
				Op:  "read",
				Err: os.ErrDeadlineExceeded,
			},
			want: terminationDetails{
				reason:    "backend_timeout",
				side:      "backend",
				operation: "read",
			},
		},
		"backend broken pipe": {
			direction: directionFrontendToBackend,
			err: &net.OpError{
				Op:  "write",
				Err: syscall.EPIPE,
			},
			want: terminationDetails{
				reason:    "backend_broken_pipe",
				side:      "backend",
				operation: "write",
			},
		},
		"gateway close": {
			direction: directionFrontendToBackend,
			err: &net.OpError{
				Op:  "read",
				Err: net.ErrClosed,
			},
			want: terminationDetails{
				reason:    "gateway_close",
				side:      "gateway",
				operation: "read",
			},
		},
		"context cancellation takes precedence": {
			direction: directionFrontendToBackend,
			err: &net.OpError{
				Op:  "read",
				Err: net.ErrClosed,
			},
			contextCancelled: true,
			want: terminationDetails{
				reason:    "context_cancelled",
				side:      "gateway",
				operation: "read",
			},
		},
		"unknown frontend error": {
			direction: directionFrontendToBackend,
			err:       errors.New("some random error"),
			want: terminationDetails{
				reason: "frontend_io_error",
				side:   "frontend",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := classifyTermination(test.direction, test.err, test.contextCancelled)
			require.Equal(t, test.want, got)
		})
	}
}

func TestAwaitCopyResult(t *testing.T) {
	t.Run("copy result", func(t *testing.T) {
		results := make(chan copyResult, 1)
		want := copyResult{direction: directionFrontendToBackend}
		results <- want

		got, received := awaitCopyResult(context.Background(), results)
		require.True(t, received)
		require.Equal(t, want, got)
	})

	t.Run("copy result before cancellation", func(t *testing.T) {
		for range 100 {
			ctx, cancel := context.WithCancel(context.Background())
			results := make(chan copyResult, 1)
			want := copyResult{direction: directionFrontendToBackend}
			results <- want
			cancel()

			got, received := awaitCopyResult(ctx, results)
			require.True(t, received)
			require.Equal(t, want, got)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		got, received := awaitCopyResult(ctx, make(chan copyResult))
		require.False(t, received)
		require.Equal(t, copyResult{}, got)
	})
}

func TestClassifyTerminationTCP(t *testing.T) {
	tests := map[string]struct {
		direction        string
		contextCancelled bool
		err              func(t *testing.T) error
		want             terminationDetails
	}{
		"frontend FIN": {
			direction: directionFrontendToBackend,
			err: func(t *testing.T) error {
				conn, peer := newTCPPair(t)
				require.NoError(t, peer.Close())
				_, err := copyLoop(io.Discard, conn)
				return err
			},
			want: terminationDetails{
				reason: "frontend_fin",
				side:   "frontend",
			},
		},
		"backend FIN": {
			direction: directionBackendToFrontend,
			err: func(t *testing.T) error {
				conn, peer := newTCPPair(t)
				require.NoError(t, peer.Close())
				_, err := copyLoop(io.Discard, conn)
				return err
			},
			want: terminationDetails{
				reason: "backend_fin",
				side:   "backend",
			},
		},
		"frontend reset": {
			direction: directionFrontendToBackend,
			err: func(t *testing.T) error {
				source, sourcePeer := newTCPPair(t)
				require.NoError(t, sourcePeer.SetLinger(0))
				require.NoError(t, sourcePeer.Close())
				_, err := copyLoop(io.Discard, source)
				return err
			},
			want: terminationDetails{
				reason:    "frontend_reset",
				side:      "frontend",
				operation: "read",
			},
		},
		"frontend TLS unexpected EOF": {
			direction: directionFrontendToBackend,
			err:       frontendTLSUnexpectedEOF,
			want: terminationDetails{
				reason:    "frontend_tls_unexpected_eof",
				side:      "frontend",
				operation: "readfrom",
			},
		},
		"backend timeout": {
			direction: directionBackendToFrontend,
			err: func(t *testing.T) error {
				conn, _ := newTCPPair(t)
				require.NoError(t, conn.SetReadDeadline(time.Now()))
				_, err := copyLoop(io.Discard, conn)
				return err
			},
			want: terminationDetails{
				reason:    "backend_timeout",
				side:      "backend",
				operation: "read",
			},
		},
		"backend broken pipe": {
			direction: directionFrontendToBackend,
			err:       brokenPipeError,
			want: terminationDetails{
				reason:    "backend_broken_pipe",
				side:      "backend",
				operation: "write",
			},
		},
		"gateway close": {
			direction: directionFrontendToBackend,
			err: func(t *testing.T) error {
				conn, _ := newTCPPair(t)
				require.NoError(t, conn.Close())
				_, err := copyLoop(io.Discard, conn)
				return err
			},
			want: terminationDetails{
				reason:    "gateway_close",
				side:      "gateway",
				operation: "read",
			},
		},
		"context cancellation": {
			direction:        directionFrontendToBackend,
			contextCancelled: true,
			err: func(t *testing.T) error {
				conn, _ := newTCPPair(t)
				require.NoError(t, conn.Close())
				_, err := copyLoop(io.Discard, conn)
				return err
			},
			want: terminationDetails{
				reason:    "context_cancelled",
				side:      "gateway",
				operation: "read",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := test.err(t)
			got := classifyTermination(test.direction, err, test.contextCancelled)
			require.Equal(t, test.want, got)
		})
	}
}

func TestServeSQLSession_ConnectionCloseUsesParentContext(t *testing.T) {
	client, inbound := net.Pipe()
	outbound, backend := net.Pipe()
	t.Cleanup(func() {
		client.Close()
		backend.Close()
	})

	var logs bytes.Buffer
	logger := zerolog.New(&logs)
	ctx := logger.WithContext(context.Background())
	tracer := noop.NewTracerProvider().Tracer("test")
	sess := New(tracer, "test-branch", inbound, outbound, nil)

	done := make(chan error, 1)
	go func() {
		done <- sess.ServeSQLSession(ctx)
	}()

	require.NoError(t, client.Close())
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("session did not complete in time")
	}

	require.Contains(t, logs.String(), `"termination_reason":"frontend_fin"`)
}

func newTCPPair(t *testing.T) (*net.TCPConn, *net.TCPConn) {
	t.Helper()

	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })

	address, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok)
	peer, err := net.DialTCP("tcp", nil, address)
	require.NoError(t, err)
	t.Cleanup(func() { peer.Close() })

	conn, err := listener.AcceptTCP()
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	return conn, peer
}

func frontendTLSUnexpectedEOF(t *testing.T) error {
	t.Helper()

	serverConn, clientConn := newTCPPair(t)
	require.NoError(t, serverConn.SetDeadline(time.Now().Add(time.Second)))
	require.NoError(t, clientConn.SetDeadline(time.Now().Add(time.Second)))

	serverTLS := tls.Server(serverConn, &tls.Config{
		Certificates: []tls.Certificate{newTestCertificate(t)},
		MinVersion:   tls.VersionTLS12,
	})
	clientTLS := tls.Client(clientConn, &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // The test uses an ephemeral self-signed certificate.
		MinVersion:         tls.VersionTLS12,
	})

	serverHandshake := make(chan error, 1)
	go func() {
		serverHandshake <- serverTLS.Handshake()
	}()
	require.NoError(t, clientTLS.Handshake())
	require.NoError(t, <-serverHandshake)

	// Write part of an application-data record, then close the transport
	// without close_notify. Go reports EOF within a TLS record as unexpected.
	_, err := clientConn.Write([]byte{0x17})
	require.NoError(t, err)
	require.NoError(t, clientConn.Close())
	destination, _ := newTCPPair(t)
	_, err = copyLoop(destination, serverTLS)
	return err
}

func newTestCertificate(t *testing.T) tls.Certificate {
	t.Helper()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, publicKey, privateKey)
	require.NoError(t, err)

	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  privateKey,
	}
}

func brokenPipeError(t *testing.T) error {
	t.Helper()

	conn, peer := newTCPPair(t)
	require.NoError(t, peer.Close())
	require.NoError(t, conn.SetWriteDeadline(time.Now().Add(time.Second)))

	payload := make([]byte, 64*1024)
	for {
		_, err := conn.Write(payload)
		if errors.Is(err, syscall.EPIPE) {
			return err
		}
		if errors.Is(err, syscall.ECONNRESET) {
			continue
		}
		require.NoError(t, err)
	}
}

func TestCopyLoop(t *testing.T) {
	tests := map[string]struct {
		reader    io.Reader
		wantErr   bool
		wantData  string
		wantBytes int64
	}{
		"successful data copy": {
			reader:    dataReader("Hello, World!"),
			wantErr:   false,
			wantData:  "Hello, World!",
			wantBytes: 13,
		},
		"EOF should not be error": {
			reader:  errorReader(io.EOF),
			wantErr: false,
		},
		"closed connection should be preserved": {
			reader:  errorReader(net.ErrClosed),
			wantErr: true,
		},
		"network reset should be preserved": {
			reader: errorReader(&net.OpError{
				Op:  "read",
				Err: syscall.ECONNRESET,
			}),
			wantErr: true,
		},
		"other network error should be error": {
			reader: errorReader(&net.OpError{
				Op:  "read",
				Err: errors.New("i/o timeout"),
			}),
			wantErr: true,
		},
		"generic IO error should be error": {
			reader:  errorReader(errors.New("some other error")),
			wantErr: true,
		},
		"bytes copied before an error are still reported": {
			reader: io.MultiReader(
				strings.NewReader("partial"),
				errorReader(errors.New("some other error")),
			),
			wantErr:   true,
			wantBytes: 7,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var writer strings.Builder
			got, err := copyLoop(&writer, test.reader)

			if test.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "copy loop")
			} else {
				require.NoError(t, err)
				if test.wantData != "" {
					require.Equal(t, test.wantData, writer.String())
				}
			}
			require.Equal(t, test.wantBytes, got)
		})
	}
}

func TestCopyLoop_AsyncClosing(t *testing.T) {
	// Use TCP connections to test async closing of any connection endpoint does exit the copy loop.

	tests := map[string]struct {
		name       string
		setupConn  func(t *testing.T) (conn net.Conn, closer func() error)
		closeDelay time.Duration
	}{
		"close local connection during copy": {
			setupConn: func(t *testing.T) (net.Conn, func() error) {
				ln, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err)

				conn, err := net.Dial("tcp", ln.Addr().String())
				require.NoError(t, err)

				conn2, err := ln.Accept()
				require.NoError(t, err)

				t.Cleanup(func() {
					conn.Close()
					conn2.Close()
					ln.Close()
				})

				return conn, conn.Close
			},
		},
		"close remote connection during copy": {
			setupConn: func(t *testing.T) (net.Conn, func() error) {
				ln, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err)

				conn, err := net.Dial("tcp", ln.Addr().String())
				require.NoError(t, err)

				conn2, err := ln.Accept()
				require.NoError(t, err)

				t.Cleanup(func() {
					conn.Close()
					conn2.Close()
					ln.Close()
				})

				return conn, conn2.Close
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			reader, closer := test.setupConn(t)

			var output strings.Builder
			errCh := make(chan error, 1)

			// Start copyLoop in goroutine
			go func() {
				_, err := copyLoop(&output, reader)
				errCh <- err
			}()

			// Close one connection endpoint after delay
			closeDelay := test.closeDelay
			if closeDelay == 0 {
				closeDelay = 50 * time.Millisecond
			}

			go func() {
				time.Sleep(closeDelay)
				closer()
			}()

			// Wait for copyLoop to complete
			select {
			case <-errCh:
			case <-time.After(200 * time.Millisecond):
				t.Fatal("copyLoop did not complete in time")
			}
		})
	}
}

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(b []byte) (int, error) {
	return f(b)
}

func errorReader(err error) io.Reader {
	return readerFunc(func(b []byte) (int, error) {
		return 0, err
	})
}

func dataReader(data string) io.Reader {
	bytes := []byte(data)
	pos := 0
	return readerFunc(func(b []byte) (int, error) {
		if pos >= len(bytes) {
			return 0, io.EOF
		}
		n := copy(b, bytes[pos:])
		pos += n
		return n, nil
	})
}

func TestReadTCPInfo(t *testing.T) {
	t.Run("returns nil for a non-socket connection", func(t *testing.T) {
		client, server := net.Pipe()
		defer client.Close()
		defer server.Close()

		info, err := readTCPInfo(client)
		require.NoError(t, err)
		require.Nil(t, info)
	})

	t.Run("nil connection is not an error", func(t *testing.T) {
		info, err := readTCPInfo(nil)
		require.NoError(t, err)
		require.Nil(t, info)
	})

	t.Run("reports acked bytes on a real socket", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer listener.Close()

		const payload = "hello backend"
		received := make(chan struct{})
		go func() {
			defer close(received)
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
			io.ReadFull(conn, make([]byte, len(payload)))
		}()

		conn, err := net.Dial("tcp", listener.Addr().String())
		require.NoError(t, err)
		defer conn.Close()

		_, err = conn.Write([]byte(payload))
		require.NoError(t, err)
		<-received

		if runtime.GOOS != "linux" {
			t.Skip("TCP_INFO is only available on Linux")
		}
		info, err := readTCPInfo(conn)
		require.NoError(t, err)
		require.NotNil(t, info)
		// The peer read everything, so the write must have been acknowledged
		// and nothing should be left outstanding.
		require.GreaterOrEqual(t, info.BytesAcked, uint64(len(payload)))
		require.Zero(t, info.NotsentBytes)
	})
}

// TestSessionCapturesBackendTCPInfo guards the ordering hazard that the
// capture must happen before the session's connections are closed. close runs
// asynchronously via the context hook, so a naive read at the end of
// ServeSQLSession finds an already-closed socket and silently reports nothing.
func TestSessionCapturesBackendTCPInfo(t *testing.T) {
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer backend.Close()

	const payload = "select 1"
	backendDone := make(chan struct{})
	go func() {
		defer close(backendDone)
		conn, err := backend.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Read what the client sent, then hang up so the session ends.
		io.ReadFull(conn, make([]byte, len(payload)))
	}()

	outbound, err := net.Dial("tcp", backend.Addr().String())
	require.NoError(t, err)

	clientSide, inbound := net.Pipe()
	go func() {
		clientSide.Write([]byte(payload))
		io.Copy(io.Discard, clientSide)
	}()

	s, ok := New(noop.NewTracerProvider().Tracer("test"), "test-branch", inbound, outbound, nil).(*session)
	require.True(t, ok)
	require.NoError(t, s.ServeSQLSession(t.Context()))
	<-backendDone

	if runtime.GOOS != "linux" {
		t.Skip("TCP_INFO is only available on Linux")
	}
	info := s.tcpInfo.Load()
	require.NotNil(t, info,
		"TCP_INFO must be captured while the backend socket is still open")
	require.GreaterOrEqual(t, info.BytesAcked, uint64(len(payload)),
		"the backend read the payload, so it must show as acknowledged")
}

// TestSetConnTCPUserTimeout exercises the inbound socket-option path used to
// bound a client that stops reading. It cannot assert the kernel timer fires
// without a slow, flaky wait, so it checks the option is accepted on a real
// socket and that non-sockets and a zero timeout are no-ops.
func TestSetConnTCPUserTimeout(t *testing.T) {
	t.Run("applies to a real socket", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer listener.Close()

		conn, err := net.Dial("tcp", listener.Addr().String())
		require.NoError(t, err)
		defer conn.Close()

		// A rejected socket option would surface here; on non-Linux the call
		// is a no-op and still returns nil.
		require.NoError(t, SetConnTCPUserTimeout(conn, 30*time.Second))
	})

	t.Run("zero timeout is a no-op", func(t *testing.T) {
		client, server := net.Pipe()
		defer client.Close()
		defer server.Close()
		require.NoError(t, SetConnTCPUserTimeout(client, 0))
	})

	t.Run("non-socket conn is a no-op", func(t *testing.T) {
		client, server := net.Pipe()
		defer client.Close()
		defer server.Close()
		// net.Pipe conns do not implement syscall.Conn, so this must not error.
		require.NoError(t, SetConnTCPUserTimeout(client, 30*time.Second))
	})
}
