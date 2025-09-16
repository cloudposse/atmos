package cmd

import (
	"errors"
	"fmt"
	"os"
)

// Main is the entry point called from the root main.go
func Main() {
	if err := Execute(); err != nil {
		// Check if it's a testFailureError
		var testErr *testFailureError
		if errors.As(err, &testErr) {
			// Print the error message and exit with the code
			fmt.Fprintf(os.Stderr, "%s\n", testErr.Error())
			os.Exit(testErr.code)
		}

		// Check if it's an exit error with a specific code
		var exitErr *exitError
		if errors.As(err, &exitErr) {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(exitErr.ExitCode())
		}

		// Default error handling
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
