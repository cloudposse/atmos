package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// validateStacksCmd validates stacks
var validateStacksCmd = &cobra.Command{
	Use:                "stacks",
	Short:              "Execute 'validate stacks' command",
	Long:               `This command validates stack configurations: atmos validate stacks`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteValidateStacksCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(err)
		}
	},
}

func init() {
	validateStacksCmd.DisableFlagParsing = false

	validateCmd.AddCommand(validateStacksCmd)
}
