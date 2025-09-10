package main

import (
	"errors"
	"fmt"
	"os"
)

func main() {
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
