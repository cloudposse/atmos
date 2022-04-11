package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// describeComponentCmd describes configuration for components
var describeStacksCmd = &cobra.Command{
	Use:                "stacks",
	Short:              "Execute 'describe stacks' command",
	Long:               `This command shows configuration for stacks and components in the stacks: atmos describe stacks <component> -s <stack>`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteDescribeStacks(cmd, args)
		if err != nil {
			u.PrintErrorToStdErrorAndExit(err)
		}
	},
}

func init() {
	describeStacksCmd.DisableFlagParsing = false
	describeStacksCmd.PersistentFlags().StringP("stack", "s", "", "atmos describe component <component> -s <stack>")

	describeCmd.AddCommand(describeStacksCmd)
}
