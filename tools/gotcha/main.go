package main

import (
	"os"

	cmd "github.com/cloudposse/atmos/tools/gotcha/cmd/gotcha"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/config"
)

func main() {
	// Initialize environment variable bindings
	config.InitEnvironment()
	
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
