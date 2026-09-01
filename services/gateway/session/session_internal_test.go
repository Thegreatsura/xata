package session

import (
	"context"
	"errors"
	"io"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestIsClosedConnError(t *testing.T) {
	tests := map[string]struct {
		err  error
		want bool
	}{
		"net.ErrClosed": {
			err:  net.ErrClosed,
			want: true,
		},
		"nil error": {
			err:  nil,
			want: false,
		},
		"non-network error": {
			err:  errors.New("some random error"),
			want: false,
		},
		"closed network connection": {
			err: &net.OpError{
				Op:  "read",
				Err: errors.New("use of closed network connection"),
			},
			want: true,
		},
		"connection reset by peer": {
			err: &net.OpError{
				Op:  "read",
				Err: errors.New("connection reset by peer"),
			},
			want: true,
		},
		"op error without wrapped error": {
			err: &net.OpError{
				Op: "read",
			},
			want: false,
		},
		"op error with other error": {
			err: &net.OpError{
				Op:  "read",
				Err: errors.New("some other network error"),
			},
			want: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := isClosedConnError(test.err)
			require.Equal(t, test.want, got)
		})
	}
}

func TestCopyLoop(t *testing.T) {
	ctx := context.Background()
	branch := "test-branch"

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
		"closed connection should not be error": {
			reader:  errorReader(net.ErrClosed),
			wantErr: false,
		},
		"network reset should be no error": {
			reader: errorReader(&net.OpError{
				Op:  "read",
				Err: errors.New("connection reset by peer"),
			}),
			wantErr: false,
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
			got, err := copyLoop(ctx, branch, &writer, test.reader)

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

	ctx := context.Background()
	branch := "test-branch"

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
				_, err := copyLoop(ctx, branch, &output, reader)
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
			case err := <-errCh:
				require.NoError(t, err)
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
