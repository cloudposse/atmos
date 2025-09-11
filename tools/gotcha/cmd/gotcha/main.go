package cmd

import (
	"errors"
	"fmt"
	"os"
)

// Main is the entry point called from the root main.go
func Main() {
	if err := Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)

		// Check if it's an exit error with a specific code
		var exitErr *exitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}

		// Default to exit code 1 for general errors
		os.Exit(1)
	}
}
