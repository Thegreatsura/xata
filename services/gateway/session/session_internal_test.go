package session

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
		reader   io.Reader
		wantErr  bool
		wantData string
	}{
		"successful data copy": {
			reader:   dataReader("Hello, World!"),
			wantErr:  false,
			wantData: "Hello, World!",
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
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var writer strings.Builder
			err := copyLoop(ctx, branch, &writer, test.reader)

			if test.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "copy loop")
			} else {
				require.NoError(t, err)
				if test.wantData != "" {
					require.Equal(t, test.wantData, writer.String())
				}
			}
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
				errCh <- copyLoop(ctx, branch, &output, reader)
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
