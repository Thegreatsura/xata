package cmd

import (
	"context"
	"fmt"
	"os"

	"xata/internal/envcfg"
	"xata/internal/o11y"
	otelruntime "xata/internal/o11y/instrumentation/runtime"
)

// WithO11y sets up observability (logging, tracing, metrics) for a command.
func WithO11y(ctx context.Context, name string, fn func(context.Context, *o11y.O) error) error {
	// Read o11y config
	var config o11y.Config
	if err := envcfg.Read(&config); err != nil {
		return fmt.Errorf("failed to read o11y config: %w", err)
	}
	monitoring := o11y.New(ctx, &config)
	defer func() {
		if err := monitoring.Shutdown(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "Error shutting down monitoring: %v", err)
		}
	}()

	o := monitoring.ForService(ctx, name, name)
	logger := o.Logger()
	defer logger.Info().Msg("Monitoring stopped")
	defer o.Close()
	defer logger.Info().Msg("Monitoring closing")

	ctx = o.WithContext(ctx)

	otelruntime.Start(&o)
	logger.Info().Msg("Monitoring started")

	return fn(ctx, &o)
}
