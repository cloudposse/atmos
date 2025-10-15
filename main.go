package main

import (
	"os"
	"os/signal"
	"syscall"

	log "github.com/cloudposse/atmos/pkg/logger"

	"github.com/cloudposse/atmos/cmd"
	errUtils "github.com/cloudposse/atmos/errors"
)

func main() {
	// Set up signal handling for graceful shutdown.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		// Clean up resources before exit.
		cmd.Cleanup()
		// Exit with correct POSIX exit code (128 + signal number).
		if s, ok := sig.(syscall.Signal); ok {
			os.Exit(128 + int(s))
		}
		// Fallback to SIGINT exit code if signal type assertion fails.
		os.Exit(130)
	}()

	// Disable timestamp in logs so snapshots work. We will address this in a future PR updating styles, etc.
	log.Default().SetReportTimestamp(false)

	// Ensure cleanup happens on normal exit.
	defer cmd.Cleanup()

	err := cmd.Execute()
	if err != nil {
		// Use GetExitCode() to extract exit code from error chain.
		// This handles ExitCodeError, exec.ExitError (terraform/helmfile), and WithExitCode errors.
		exitCode := errUtils.GetExitCode(err)

		// Format and print the error using the error formatter.
		formatted := errUtils.Format(err, errUtils.DefaultFormatterConfig())
		if formatted != "" {
			os.Stderr.WriteString(formatted)
			os.Stderr.WriteString("\n")
		}

		log.Debug("Exiting with code", "code", exitCode)
		os.Exit(exitCode) //nolint:revive // main() is allowed to call os.Exit
	}
}
