package main

import (
	"fmt"
	"os"

	"xata/internal/cmd"
	"xata/services/projects"
)

func main() {
	svc := projects.NewProjectsService()
	rootCmd := cmd.RootCmdForService(svc)
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err) //nolint:forbidigo
		os.Exit(1)
	}
}
