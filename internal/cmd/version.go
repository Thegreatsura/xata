package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"xata/internal/o11y/version"
	"xata/internal/service"
)

func versionCmd(svc service.Service) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version info",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(svc.Name() + " service - version " + version.Get()) //nolint:forbidigo
		},
	}
}
