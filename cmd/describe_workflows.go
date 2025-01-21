package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// describeWorkflowsCmd executes 'atmos describe workflows' CLI commands
var describeWorkflowsCmd = &cobra.Command{
	Use:   "workflows",
	Short: "List Atmos workflows and their associated files",
	Long:  "List all Atmos workflows, showing their associated files and workflow names for easy reference.",
	Example: "describe workflows\n" +
		"describe workflows --format json\n" +
		"describe workflows -f yaml\n" +
		"describe workflows --output list\n" +
		"describe workflows -o map -f json\n" +
		"describe workflows -o map\n" +
		"describe workflows -o all",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteDescribeWorkflowsCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(err)
		}
	},
}

func init() {
	describeWorkflowsCmd.PersistentFlags().StringP("format", "f", "yaml", "Specify the output format: atmos describe workflows --format=<yaml|json> ('yaml' is default)")
	describeWorkflowsCmd.PersistentFlags().StringP("output", "o", "list", "Specify the output type: atmos describe workflows --output=<list|map|all> ('list' is default)")

	describeCmd.AddCommand(describeWorkflowsCmd)
}
