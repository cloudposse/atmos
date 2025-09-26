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
		<-sigChan
		// Clean up resources before exit.
		cmd.Cleanup()
		os.Exit(130) // Standard exit code for SIGINT.
	}()

	// Disable timestamp in logs so snapshots work. We will address this in a future PR updating styles, etc.
	log.Default().SetReportTimestamp(false)

	// Ensure cleanup happens on normal exit.
	defer cmd.Cleanup()

	err := cmd.Execute()
	if err != nil {
		if errors.Is(err, errUtils.ErrPlanHasDiff) {
			log.Debug("Exiting with code 2 due to plan differences")
			errUtils.Exit(2)
		}
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}
}
