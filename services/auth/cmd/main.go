package main

import (
	"fmt"
	"os"

	"xata/internal/cmd"
	"xata/services/auth"
	"xata/services/auth/devuser"
)

func main() {
	svc := auth.NewAuthService()
	cmd := cmd.RootCmdForService(svc)
	cmd.AddCommand(devuser.CreateDevUserCmd())

	if err := cmd.Execute(); err != nil {
		fmt.Println(err) //nolint:forbidigo
		os.Exit(1)
	}
}
