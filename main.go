package main

import (
	"errors"

	log "github.com/cloudposse/atmos/pkg/logger"

	"github.com/cloudposse/atmos/cmd"
	errUtils "github.com/cloudposse/atmos/errors"
)

func main() {
	// Disable timestamp in logs so snapshots work. We will address this in a future PR updating styles, etc.
	log.Default().SetReportTimestamp(false)

	err := cmd.Execute()
	if err != nil {
		if errors.Is(err, errUtils.ErrPlanHasDiff) {
			log.Debug("Exiting with code 2 due to plan differences")
			errUtils.Exit(2)
		}
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}
}
