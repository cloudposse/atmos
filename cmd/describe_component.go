package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
	"os"
)

// describeComponentCmd describes configuration for components
var describeComponentCmd = &cobra.Command{
	Use:                "component",
	Short:              "Execute 'describe component' command",
	Long:               `This command shows configuration for a component in a stack: atmos describe component <component> -s <stack>`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteDescribeComponent(cmd, args)
		if err != nil {
			os.Exit(1)
		}
	},
}

func init() {
	describeComponentCmd.DisableFlagParsing = false
	describeComponentCmd.PersistentFlags().StringP("stack", "s", "", "atmos describe component <component> -s <stack>")

	err := describeComponentCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		u.PrintErrorToStdErrorAndExit(err)
	}

	describeCmd.AddCommand(describeComponentCmd)
}
