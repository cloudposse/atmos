package main

import (
	"os"

	"github.com/cloudposse/atmos/tools/director/cmd"
)

func main() {
	// Fang already handles error styling and output
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
