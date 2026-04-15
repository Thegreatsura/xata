package main

import (
	"fmt"
	"os"

	"xata/internal/cmd"
	"xata/services/gateway"
)

func main() {
	cliConfig := gateway.CLIConfig{}
	svc := gateway.NewGatewayService(&cliConfig)

	rootCmd := cmd.RootCmdForService(svc)
	rootCmd.PersistentFlags().StringVar(&cliConfig.DevPostgresURL, "dev-postgres-url", "", "development postgres connection string. If set the gateway will always route to this postgres instance.")
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err) //nolint:forbidigo
		os.Exit(1)
	}
}
