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
	Long:               `This command shows configuration for stacks and components in the stacks: atmos describe stacks`,
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
	describeStacksCmd.PersistentFlags().String("format", "yaml", "atmos describe stacks --format=yaml/json")
	describeStacksCmd.PersistentFlags().StringP("file", "f", "", "atmos describe stacks --file=stacks.yaml")

	describeCmd.AddCommand(describeStacksCmd)
}
