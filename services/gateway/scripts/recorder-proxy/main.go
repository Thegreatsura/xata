package main

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"xata/services/gateway/scripts/internal/bench"

	"github.com/elastic/go-concert/ctxtool"
	"github.com/elastic/go-concert/unison"
	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/rs/zerolog"
)

var (
	recordAuth     bool
	listenAddress  string
	outputHostPort string
	outfile        string
	compress       bool
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	flag.BoolVar(&recordAuth, "record-auth", false, "record authentication messages")
	flag.StringVar(&listenAddress, "listen", ":6543", "listen address")
	flag.StringVar(&outputHostPort, "output-host-port", "localhost:5432", "output host and port")
	flag.StringVar(&outfile, "out", "", "output file")
	flag.BoolVar(&compress, "compress", false, "compress output")
	flag.Parse()

	var ac ctxtool.AutoCancel
	defer ac.Cancel()

	ctx := ac.With(signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM))

	logger := zerolog.New(os.Stderr).With().Str("component", "recorder").Logger()
	ctx = logger.WithContext(ctx)

	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		return fmt.Errorf("failed to bind port: %w", err)
	}
	ac.With(ctxtool.WithFunc(ctx, func() {
		logger.Info().Msg("Listener shutdown signal received")
		listener.Close()
	}))

	var out io.Writer = os.Stdout
	if outfile != "" {
		outFile, err := os.Create(outfile)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer outFile.Close()

		out = outFile
	}

	if compress {
		gzipWriter := gzip.NewWriter(out)
		defer gzipWriter.Close()
		out = gzipWriter
	}

	dialer := &net.Dialer{}

	// we handle only one connection at a time. No need to create goroutines
	for ctx.Err() == nil {
		err := handleNextClient(ctx, out, listener, dialer, outputHostPort)
		if err != nil {
			logger.Error().Err(err).Msg("failed to handle next client")
		}
	}

	return nil
}

func handleNextClient(
	ctx context.Context,
	out io.Writer,
	listener net.Listener,
	dialer *net.Dialer,
	outAddress string,
) error {
	clientConn, clientCancel, err := accept(ctx, listener)
	if err != nil {
		return fmt.Errorf("failed to accept client connection: %w", err)
	}
	defer clientCancel()

	endpoint, endpointCancel, err := dial(ctx, dialer, outAddress)
	if err != nil {
		return fmt.Errorf("failed to dial endpoint: %w", err)
	}
	defer endpointCancel()

	var encLock sync.Mutex
	encoder := json.NewEncoder(out)
	reporter := func(msg any) {
		encLock.Lock()
		defer encLock.Unlock()
		encoder.Encode(msg)
	}

	authReporter := reporter
	if !recordAuth {
		authReporter = func(any) {}
	}

	backend := pgproto3.NewBackend(clientConn, clientConn)
	frontend := pgproto3.NewFrontend(endpoint, endpoint)
	if err := authenticate(authReporter, backend, frontend); err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	tg := unison.TaskGroupWithCancel(ctx)
	tg.Go(func(ctx context.Context) error {
		return copyLoop(ctx, backend, frontend, reporter)
	})
	tg.Go(func(ctx context.Context) error {
		return copyLoop(ctx, frontend, backend, reporter)
	})
	return tg.Wait()
}

func copyLoop[M any, F bench.Receiver[M], T bench.Sender[M]](
	ctx context.Context,
	from F,
	to T,
	reporter func(any),
) error {
	var err error
	for ctx.Err() == nil {
		var msg M
		msg, err = from.Receive()
		if err != nil {
			err = fmt.Errorf("failed to receive message: %w", err)
			break
		}

		to.Send(msg)
		if err = to.Flush(); err != nil {
			err = fmt.Errorf("failed to flush message: %w", err)
			break
		}

		reporter(msg)
	}

	if err != nil {
		if isClosedConnError(err) {
			return nil
		}
		return err
	}

	return nil
}

func isClosedConnError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
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

func authenticate(
	reporter func(any),
	backend *pgproto3.Backend,
	frontend *pgproto3.Frontend,
) error {
	inAuth := false

	for {
		var err error
		var startupMsg pgproto3.FrontendMessage

		if inAuth {
			startupMsg, err = backend.Receive()
		} else {
			startupMsg, err = backend.ReceiveStartupMessage()
		}
		if err != nil {
			return fmt.Errorf("failed to receive startup message: %w", err)
		}

		reporter(startupMsg)

		switch m := startupMsg.(type) {
		case *pgproto3.StartupMessage:
			frontend.Send(m)
		case *pgproto3.PasswordMessage:
			frontend.Send(m)
		case *pgproto3.Terminate:
			frontend.Send(m)
			frontend.Flush()
			return nil
		default:
			return fmt.Errorf("unexpected message type, only StartupMessage and PasswordMessage are supported: %T", m)
		}

		if err := frontend.Flush(); err != nil {
			return fmt.Errorf("failed to forward message to postgres: %w", err)
		}

	waitReadyLoop:
		for {
			startupResp, err := frontend.Receive()
			if err != nil {
				return fmt.Errorf("failed to receive startup response: %w", err)
			}

			reporter(startupResp)

			backend.Send(startupResp)
			if err := backend.Flush(); err != nil {
				return fmt.Errorf("failed to return response to client: %w", err)
			}

			switch startupResp.(type) {
			// Init authentication
			case *pgproto3.AuthenticationCleartextPassword,
				*pgproto3.AuthenticationMD5Password,
				*pgproto3.AuthenticationSASL:
				inAuth = true
				break waitReadyLoop
			// Other startup message
			case *pgproto3.AuthenticationOk, *pgproto3.ParameterStatus, *pgproto3.BackendKeyData:
			case *pgproto3.ReadyForQuery:
				// yay
				return nil

			default:
				return fmt.Errorf("unexpected message type. Only password auth support, received %T", startupResp)
			}
		}
	}
}

func accept(ctx context.Context, listener net.Listener) (net.Conn, context.CancelFunc, error) {
	clientConn, err := listener.Accept()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to accept connection: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	go func() {
		<-ctx.Done()
		clientConn.Close()
	}()
	return clientConn, cancel, nil
}

func dial(ctx context.Context, dialer *net.Dialer, address string) (net.Conn, context.CancelFunc, error) {
	endpoint, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to dial endpoint: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	go func() {
		<-ctx.Done()
		endpoint.Close()
	}()
	return endpoint, cancel, nil
}
