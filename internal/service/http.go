package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"xata/internal/api"
	"xata/internal/o11y"
)

// RunHTTPService runs the given HTTPServices
// It will start the HTTP server and block until the context is cancelled
func RunHTTPService(ctx context.Context, o *o11y.O, svc ...HTTPService) error {
	router := api.SetupRouter(o)
	logger := o.Logger()
	for _, s := range svc {
		if err := s.RegisterHTTPHandlers(o, router.Group("")); err != nil {
			return errors.New("failed to register HTTP handlers")
		}
	}

	addr := "0.0.0.0:5001"
	logger.Info().Str("addr", addr).Msg("starting API server")
	startErrChan := make(chan error, 1)
	defer close(startErrChan)
	go func() {
		if err := router.Start(addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			startErrChan <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info().Msg("shuting down API server")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := router.Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown API server: %w", err)
		}
		return nil
	case err := <-startErrChan:
		return fmt.Errorf("API server failure: %w", err)
	}
}
