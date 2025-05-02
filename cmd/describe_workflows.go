package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// describeWorkflowsCmd executes 'atmos describe workflows' CLI commands
var describeWorkflowsCmd = &cobra.Command{
	Use:                "workflows",
	Short:              "List Atmos workflows and their associated files",
	Long:               "List all Atmos workflows, showing their associated files and workflow names for easy reference.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteDescribeWorkflowsCmd(cmd, args)
		if err != nil {
			u.PrintErrorMarkdownAndExit("", err, "")
		}
	},
}

func init() {
	describeWorkflowsCmd.PersistentFlags().StringP("format", "f", "yaml", "Specify the output format (`yaml` is default)")
	describeWorkflowsCmd.PersistentFlags().StringP("output", "o", "list", "Specify the output type (`list` is default)")

	describeCmd.AddCommand(describeWorkflowsCmd)
}
