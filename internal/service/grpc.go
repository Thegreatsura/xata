package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/rs/zerolog/log"

	"xata/internal/api"
	"xata/internal/envcfg"
	"xata/internal/o11y"
)

// RunGRPCService runs the given GRPCService
// It will start the gRPC server and block until the context is cancelled
func RunGRPCService(ctx context.Context, o *o11y.O, svc ...GRPCService) error {
	// Read the configuration
	var config api.Config
	err := envcfg.Read(&config)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	server := api.SetupGRPC(o, config)

	for _, s := range svc {
		s.RegisterGRPCHandlers(o, server)
	}

	var addr string
	if addr == "" {
		addr = "0.0.0.0:5002"
	}

	// Listen and Serve
	logger := log.Ctx(ctx)
	logger.Info().Str("addr", addr).Msg("Starting gRPC server")

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	defer lis.Close()

	go func() {
		if err := server.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Err(err).Msg("gRPC server failure")
		}
	}()

	<-ctx.Done()

	logger.Info().Msg("Shuting down gRPC server")
	server.GracefulStop()

	return nil
}
