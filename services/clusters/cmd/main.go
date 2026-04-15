package main

import (
	"fmt"
	"os"

	"xata/internal/cmd"
	"xata/services/clusters"
)

func main() {
	svc := clusters.NewClustersService()
	if err := cmd.RootCmdForService(svc).Execute(); err != nil {
		fmt.Println(err) //nolint:forbidigo
		os.Exit(1)
	}
}
