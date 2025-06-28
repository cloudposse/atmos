package main

import (
	"errors"

	log "github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/cmd"
	errUtils "github.com/cloudposse/atmos/errors"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func main() {
	// Disable timestamp in logs so snapshots work. We will address this in a future PR updating styles, etc.
	log.Default().SetReportTimestamp(false)

	err := cmd.Execute()
	if err != nil {
		if errors.Is(err, errUtils.ErrPlanHasDiff) {
			log.Debug("Exiting with code 2 due to plan differences")
			u.OsExit(2)
		}
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}
}
