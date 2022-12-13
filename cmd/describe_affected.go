package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// describeAffectedCmd produces a list of the affected Atmos components and stacks given two Git commits
var describeAffectedCmd = &cobra.Command{
	Use:                "affected",
	Short:              "Execute 'describe affected' command",
	Long:               `This command produces a list of the affected Atmos components and stacks given two Git commits: atmos describe stacks [options]`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteDescribeAffectedCmd(cmd, args)
		if err != nil {
			u.PrintErrorToStdErrorAndExit(err)
		}
	},
}

func init() {
	describeAffectedCmd.DisableFlagParsing = false

	describeAffectedCmd.PersistentFlags().String("base", "", "The SHA of a Git commit to compare the current Git checkout to: atmos describe affected --base origin/main")
	describeAffectedCmd.PersistentFlags().String("file", "", "Write the result to file: atmos describe affected --base origin/main --file affected.json")
	describeAffectedCmd.PersistentFlags().String("format", "json", "The output format: atmos describe affected --base origin/main --format=json|yaml ('json' is default)")

	err := describeAffectedCmd.MarkPersistentFlagRequired("base")
	if err != nil {
		u.PrintErrorToStdErrorAndExit(err)
	}

	describeCmd.AddCommand(describeAffectedCmd)
}
