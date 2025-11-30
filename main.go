package main

import (
	"errors"
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
		// Check for typed exit code error first to preserve subcommand exit codes.
		var exitCodeErr errUtils.ExitCodeError
		if errors.As(err, &exitCodeErr) {
			log.Debug("Exiting with subcommand exit code", "code", exitCodeErr.Code)
			errUtils.Exit(exitCodeErr.Code)
		}
		if errors.Is(err, errUtils.ErrPlanHasDiff) {
			log.Debug("Exiting with code 2 due to plan differences")
			errUtils.Exit(2)
		}
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}
}
