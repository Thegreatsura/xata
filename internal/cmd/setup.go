package cmd

import (
	"context"

	"github.com/spf13/cobra"

	"xata/internal/o11y"
	"xata/internal/service"
)

func setupCmd(svc service.Service) *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Setup the service (e.g. create database tables)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return WithO11y(cmd.Context(), svc.Name(), func(ctx context.Context, o *o11y.O) error {
				return svc.Setup(ctx)
			})
		},
	}
}
