package main

import (
	"errors"

	"github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/cmd"
	terrerrors "github.com/cloudposse/atmos/pkg/errors"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func main() {
	// Disable timestamp in logs so snapshots work. We will address this in a future PR updating styles, etc.
	log.Default().SetReportTimestamp(false)

	err := cmd.Execute()
	if err != nil {
		if errors.Is(err, terrerrors.ErrPlanHasDiff) {
			log.Debug("Exiting with code 2 due to plan differences")
			u.OsExit(2)
		}
		u.LogErrorAndExit(err)
	}
}
