package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// describeWorkflowsCmd executes 'atmos describe workflows' CLI commands
var describeWorkflowsCmd = &cobra.Command{
	Use:   "workflows",
	Short: "Display information about Atmos workflows",
	Long:  "This command provides details about the Atmos workflows, helping you review and understand the workflows configured in the system.",
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
			u.LogErrorAndExit(schema.CliConfiguration{}, err)
		}
	},
}

func init() {
	describeWorkflowsCmd.PersistentFlags().StringP("format", "f", "yaml", "Specify the output format: atmos describe workflows --format=<yaml|json> ('yaml' is default)")
	describeWorkflowsCmd.PersistentFlags().StringP("output", "o", "list", "Specify the output type: atmos describe workflows --output=<list|map|all> ('list' is default)")

	describeCmd.AddCommand(describeWorkflowsCmd)
}
