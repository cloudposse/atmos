package cmd

import (
	"errors"
)

// Main is the entry point called from the root main.go.
// It returns the exit code that should be used.
func Main() int {
	if err := Execute(); err != nil {
		// Fang will have already printed the error in its nice format
		// We just need to handle the exit code

		// Check if it's a testFailureError
		var testErr *testFailureError
		if errors.As(err, &testErr) {
			return testErr.code
		}

		// Check if it's an exit error with a specific code
		var exitErr *exitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}

		// Default error handling
		return 1
	}
	return 0
}
