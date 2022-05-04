package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// describeStacksCmd describes configuration for stacks and components in the stacks
var validateStacksCmd = &cobra.Command{
	Use:                "stacks",
	Short:              "Execute 'validate stacks' command",
	Long:               `This command validates stack configurations: atmos validate stacks <options>`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteDescribeStacks(cmd, args)
		if err != nil {
			u.PrintErrorToStdErrorAndExit(err)
		}
	},
}

func init() {
	validateStacksCmd.DisableFlagParsing = false

	validateCmd.AddCommand(validateStacksCmd)
}
