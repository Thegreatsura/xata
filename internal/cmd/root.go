package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"xata/internal/service"
)

// RootCmdForService returns a cobra command for the given service
func RootCmdForService(svc service.Service) *cobra.Command {
	run := runCmd(svc)

	cmd := &cobra.Command{
		Use:   svc.Name(),
		Short: svc.Name() + " service, run without arguments to start the service",
		RunE:  run.RunE, // default to run command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// read config before running the command
			err := svc.ReadConfig(cmd.Context())
			if err != nil {
				return fmt.Errorf("error reading config: %w", err)
			}

			// init service before running any command
			err = svc.Init(cmd.Context())
			if err != nil {
				return fmt.Errorf("error initializing service: %w", err)
			}

			return nil
		},
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			return svc.Close(cmd.Context())
		},
	}

	cmd.AddCommand(run)
	cmd.AddCommand(versionCmd(svc))
	cmd.AddCommand(setupCmd(svc))

	return cmd
}
