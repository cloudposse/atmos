package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// describeAffectedCmd produces a list of the affected Atmos components and stacks given two Git commits
var describeDependantsCmd = &cobra.Command{
	Use:                "dependants",
	Short:              "Execute 'describe dependants' command",
	Long:               `This command produces a list of Atmos components in Atmos stacks that depend on the provided Atmos component: atmos describe dependants [options]`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteDescribeDependantsCmd(cmd, args)
		if err != nil {
			u.PrintErrorToStdErrorAndExit(err)
		}
	},
}

func init() {
	describeDependantsCmd.DisableFlagParsing = false

	describeDependantsCmd.PersistentFlags().StringP("stack", "s", "", "atmos describe dependants <component> -s <stack>")
	describeDependantsCmd.PersistentFlags().StringP("format", "f", "json", "The output format: atmos describe dependants <component> -s <stack> --format=json|yaml ('json' is default)")
	describeDependantsCmd.PersistentFlags().String("file", "", "Write the result to the file: atmos describe dependants <component> -s <stack> --file dependants.yaml")

	err := describeDependantsCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		u.PrintErrorToStdErrorAndExit(err)
	}

	describeCmd.AddCommand(describeDependantsCmd)
}
