package main

import (
	"os"

	cmd "github.com/cloudposse/atmos/tools/gotcha/cmd/gotcha"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}