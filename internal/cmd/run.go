package cmd

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/elastic/go-concert/ctxtool"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"xata/internal/o11y"
	"xata/internal/service"
)

func runCmd(svc service.Service) *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run the service",
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO move o11y setup to a common place
			var ctxCancel ctxtool.AutoCancel
			defer ctxCancel.Cancel()

			ctx := ctxCancel.With(signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM))

			return WithO11y(ctx, svc.Name(), func(ctx context.Context, o *o11y.O) error {
				logger := o.Logger()
				logger.Info().Msg("starting service")
				defer logger.Info().Msg("service stopped")

				// Setup shutdown signal handler
				ctx = ctxCancel.With(ctxtool.WithFunc(ctx, func() {
					logger.Info().Msg("shutdown signal received")
				}))

				eg, ctx := errgroup.WithContext(ctx)

				// Run HTTP service if available
				if httpSvc, ok := svc.(service.HTTPService); ok {
					eg.Go(func() error {
						return service.RunHTTPService(ctx, o, httpSvc)
					})
				}

				// Run gRPC service if available
				if grpcSvc, ok := svc.(service.GRPCService); ok {
					eg.Go(func() error {
						return service.RunGRPCService(ctx, o, grpcSvc)
					})
				}

				// Run generic service if available
				if genericSvc, ok := svc.(service.RunnerService); ok {
					eg.Go(func() error {
						return service.RunGenericService(ctx, o, genericSvc)
					})
				}

				return eg.Wait()
			})
		},
	}
}
