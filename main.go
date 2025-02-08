package main

import (
	"github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/cmd"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func main() {
	// Disable timestamp in logs so snapshots work. We will address this in a future PR updating styles, etc.
	log.Default().SetReportTimestamp(false)

	err := cmd.Execute()
	if err != nil {
		u.LogErrorAndExit(err)
	}
}
