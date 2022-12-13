package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// describeAffectedCmd shows the affected (changed) configurations for Atmos stacks and components in the stacks
var describeAffectedCmd = &cobra.Command{
	Use:                "affected",
	Short:              "Execute 'describe affected' command",
	Long:               `This command shows the affected (changed) configurations for Atmos stacks and components in the stacks: atmos describe stacks [options]`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteDescribeStacksCmd(cmd, args)
		if err != nil {
			u.PrintErrorToStdErrorAndExit(err)
		}
	},
}

func init() {
	describeAffectedCmd.DisableFlagParsing = false

	describeAffectedCmd.PersistentFlags().String("file", "", "Write the result to file: atmos describe affected --file=affected.yaml")
	describeAffectedCmd.PersistentFlags().String("format", "yaml", "Specify the output format: atmos describe affected --format=json|yaml ('json' is default)")

	describeCmd.AddCommand(describeAffectedCmd)
}
