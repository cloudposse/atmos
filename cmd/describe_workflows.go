package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// describeWorkflowsCmd executes 'atmos describe workflows' CLI commands
var describeWorkflowsCmd = &cobra.Command{
	Use:                "workflows",
	Short:              "Execute 'describe workflows' commands",
	Long:               `This command executes 'atmos describe workflows' CLI command`,
	Example:            "describe workflows\ndescribe workflows --format json\ndescribe workflows -f yaml",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteDescribeWorkflowsCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(err)
		}
	},
}

func init() {
	describeWorkflowsCmd.PersistentFlags().StringP("format", "f", "yaml", "Specify the output format: atmos describe stacks --format=yaml|json ('yaml' is default)")

	describeCmd.AddCommand(describeWorkflowsCmd)
}
