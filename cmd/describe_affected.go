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

	describeAffectedCmd.PersistentFlags().String("ref", "", "Git reference with which to compare the current branch: atmos describe affected --ref refs/heads/main. Refer to https://git-scm.com/book/en/v2/Git-Internals-Git-References for more details")
	describeAffectedCmd.PersistentFlags().String("sha", "", "Git commit SHA with which to compare the current branch: atmos describe affected --sha 3a5eafeab90426bd82bf5899896b28cc0bab3073")
	describeAffectedCmd.PersistentFlags().String("file", "", "Write the result to the file: atmos describe affected --ref refs/tags/v1.16.0 --file affected.json")
	describeAffectedCmd.PersistentFlags().String("format", "json", "The output format: atmos describe affected --ref refs/heads/main --format=json|yaml ('json' is default)")
	describeAffectedCmd.PersistentFlags().Bool("verbose", false, "atmos describe affected --ref refs/heads/main --verbose=true")

	describeCmd.AddCommand(describeAffectedCmd)
}
