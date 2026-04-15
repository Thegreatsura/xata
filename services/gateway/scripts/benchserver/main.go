package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"xata/services/gateway/scripts/internal/bench"

	"github.com/elastic/go-concert/ctxtool"
	"github.com/elastic/go-concert/unison"
	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	if err := run(); err != nil {
		log.Fatal().Err(err).Msg("Error")
	}
}

func run() error {
	var listenAddress string
	var scriptFolder string
	var validate bool
	var debug bool
	flag.StringVar(&listenAddress, "l", "127.0.0.1:5432", "address to listen on")
	flag.StringVar(&scriptFolder, "s", ".", "folder containing scripts")
	flag.BoolVar(&validate, "validate", false, "validate received messages")
	flag.BoolVar(&debug, "d", false, "debug mode")
	flag.Parse()

	compileFrontendStep := createFrontendStep
	if validate {
		compileFrontendStep = createValidatingFrontendStep
	}
	scripts, err := preloadScripts(context.Background(), scriptFolder, bench.BackendLoadConfig{
		CompileFrontendStep: compileFrontendStep,
		CompileBackendStep:  createBackendStep,
	})
	if err != nil {
		return fmt.Errorf("preload scripts: %w", err)
	}

	var ac ctxtool.AutoCancel
	defer ac.Cancel()

	ctx := ac.With(signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM))

	logLevel := zerolog.InfoLevel
	if debug {
		logLevel = zerolog.DebugLevel
	}
	logger := zerolog.New(os.Stderr).Level(logLevel).With().Timestamp().Logger()
	ctx = logger.WithContext(ctx)

	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	ctx = ac.With(ctxtool.WithFunc(ctx, func() {
		logger.Info().Msg("Shutting down")
		listener.Close()
	}))

	loader := func(ctx context.Context, scriptName string) (bench.BackendScript, error) {
		script, ok := scripts[scriptName]
		if !ok {
			return nil, fmt.Errorf("script %q not found", scriptName)
		}
		return script, nil
	}
	server := newServer(listener, loader)
	return server.run(ctx)
}

type server struct {
	listener net.Listener
	loader   scriptLoader
}

type scriptLoader func(ctx context.Context, scriptName string) (bench.BackendScript, error)

func newServer(listener net.Listener, loader scriptLoader) *server {
	return &server{
		listener: listener,
		loader:   loader,
	}
}

func (s *server) run(ctx context.Context) error {
	logger := zerolog.Ctx(ctx).With().Str("server", s.listener.Addr().String()).Logger()
	ctx = logger.WithContext(ctx)

	logger.Info().Msg("Server started")
	defer logger.Info().Msg("Server stopped")

	tg := unison.TaskGroupWithCancel(ctx)
	for ctx.Err() == nil {
		conn, err := s.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				break
			}
			return fmt.Errorf("accept: %w", err)
		}
		tg.Go(func(ctx context.Context) error {
			logger := zerolog.Ctx(ctx).With().Str("client", conn.RemoteAddr().String()).Logger()
			ctx = logger.WithContext(ctx)

			err := handleClient(ctx, conn, s.loader)
			if err != nil {
				logger.Error().Err(err).Msg("Client error")
			}
			return nil
		})
	}
	return tg.Wait()
}

func handleClient(ctx context.Context, conn net.Conn, loader scriptLoader) error {
	logger := zerolog.Ctx(ctx)

	logger.Info().Msg("Client connected")
	defer logger.Info().Msg("Client disconnected")

	ctx, cancel := ctxtool.WithFunc(ctx, func() {
		logger.Debug().Msg("Shutting down client process")
		conn.Close()
	})
	defer cancel()

	backend := pgproto3.NewBackend(conn, conn)
	script, err := processConnectionSetup(ctx, backend, loader)
	if err != nil {
		return fmt.Errorf("process startup: %w", err)
	}

	backend.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
	if err := backend.Flush(); err != nil {
		return fmt.Errorf("flush ready for query: %w", err)
	}

	logger.Info().Msg("Running script")
	defer logger.Info().Msg("Script finished")

	for ctx.Err() == nil {
		if err := script.Run(ctx, backend); err != nil {
			return fmt.Errorf("run script: %w", err)
		}
	}
	return nil
}

func processConnectionSetup(ctx context.Context, backend *pgproto3.Backend, loader scriptLoader) (bench.BackendScript, error) {
	logger := zerolog.Ctx(ctx)

	logger.Debug().Msg("Waiting for startup message")
	startupMsg, err := backend.ReceiveStartupMessage()
	if err != nil {
		return nil, fmt.Errorf("receive startup message: %w", err)
	}
	logger.Debug().Interface("startupMsg", startupMsg).Msg("Received startup message")

	switch startupMsg.(type) {
	case *pgproto3.SSLRequest:
		return nil, fmt.Errorf("TLS encryption not supported")
	case *pgproto3.GSSEncRequest:
		return nil, fmt.Errorf("GSS encryption not supported")
	case *pgproto3.StartupMessage:
		// Request plaintext password authentication
		logger.Debug().Msg("Requesting cleartext password authentication")
		backend.Send(&pgproto3.AuthenticationCleartextPassword{})
		if err := backend.Flush(); err != nil {
			return nil, fmt.Errorf("flush auth request: %w", err)
		}

		// Wait for password message and read script name
		msg, err := backend.Receive()
		if err != nil {
			return nil, fmt.Errorf("receive password: %w", err)
		}
		passwordMsg, ok := msg.(*pgproto3.PasswordMessage)
		if !ok {
			return nil, fmt.Errorf("expected password message, got %T", msg)
		}
		scriptName := string(passwordMsg.Password)
		script, err := loader(ctx, scriptName)
		if err != nil {
			return nil, fmt.Errorf("load script %q: %w", scriptName, err)
		}

		// Send auth OK
		backend.Send(&pgproto3.AuthenticationOk{})
		if err := backend.Flush(); err != nil {
			return nil, fmt.Errorf("flush auth ok: %w", err)
		}

		return script, nil
	default:
		return nil, fmt.Errorf("unexpected startup message type: %T", startupMsg)
	}
}

func createFrontendStep(_ pgproto3.FrontendMessage) (bench.BackendStep, error) {
	return bench.AwaitMessages[pgproto3.FrontendMessage, pgproto3.BackendMessage](1), nil
}

func createValidatingFrontendStep(msg pgproto3.FrontendMessage) (bench.BackendStep, error) {
	return bench.AwaitValidateMessage[pgproto3.FrontendMessage, pgproto3.BackendMessage](msg), nil
}

func createBackendStep(msg pgproto3.BackendMessage) (bench.BackendStep, error) {
	emsg, err := bench.NewEncodedMessage(msg)
	if err != nil {
		return nil, fmt.Errorf("encode message: %w", err)
	}
	return bench.SendMessages[pgproto3.FrontendMessage, pgproto3.BackendMessage](emsg), nil
}

func preloadScripts(ctx context.Context, scriptFolder string, config bench.BackendLoadConfig) (map[string]bench.BackendScript, error) {
	entries, err := os.ReadDir(scriptFolder)
	if err != nil {
		return nil, fmt.Errorf("read scripts: %w", err)
	}

	scripts := make(map[string]bench.BackendScript)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		log.Ctx(ctx).Info().Str("script", entry.Name()).Msg("Loading script")

		name := entry.Name()
		script, err := bench.LoadScript(ctx, scriptFolder, name, config)
		if err != nil {
			return nil, fmt.Errorf("load script %q: %w", name, err)
		}
		scripts[name] = script
	}

	if len(scripts) == 0 {
		return nil, fmt.Errorf("no scripts found")
	}

	return scripts, nil
}
