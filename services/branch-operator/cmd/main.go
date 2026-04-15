package main

import (
	"fmt"
	"os"

	"xata/internal/cmd"
	branchoperator "xata/services/branch-operator"
)

func main() {
	svc := branchoperator.NewBranchOperatorService()
	if err := cmd.RootCmdForService(svc).Execute(); err != nil {
		fmt.Println(err) //nolint:forbidigo
		os.Exit(1)
	}
}
