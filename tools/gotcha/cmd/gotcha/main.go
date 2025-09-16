package cmd

import (
	"errors"
	"os"
)

// Main is the entry point called from the root main.go.
func Main() {
	if err := Execute(); err != nil {
		// Fang will have already printed the error in its nice format
		// We just need to handle the exit code

		// Check if it's a testFailureError
		var testErr *testFailureError
		if errors.As(err, &testErr) {
			os.Exit(testErr.code)
		}

		// Check if it's an exit error with a specific code
		var exitErr *exitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}

		// Default error handling
		os.Exit(1)
	}
}
