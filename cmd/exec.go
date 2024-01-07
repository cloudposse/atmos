package cmd

import (
	"github.com/spf13/cobra"

	tui "github.com/cloudposse/atmos/internal/tui"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// execCmd produces a list of the affected Atmos components and stacks given two Git commits
var execCmd = &cobra.Command{
	Use:                "exec",
	Short:              "Execute 'exec' command",
	Long:               `This command launches TUI that allows to interactively select an Atmos component and stack, and a command to execute`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		err := tui.ExecuteExecCmd3(cmd, args)
		if err != nil {
			u.LogErrorAndExit(err)
		}
	},
}

func init() {
	RootCmd.AddCommand(execCmd)
}
