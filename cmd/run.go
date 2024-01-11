package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// runCmd launches TUI that allows to interactively select an Atmos component and stack, and a command to execute
var runCmd = &cobra.Command{
	Use:                "run",
	Short:              "Execute 'run' command",
	Long:               `This command launches TUI that allows to interactively select an Atmos component and stack, and a command to execute`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Example:            "atmos run",
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteRunCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(err)
		}
	},
}

func init() {
	RootCmd.AddCommand(runCmd)
}
