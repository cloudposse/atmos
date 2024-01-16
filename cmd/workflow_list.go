package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// workflowListCmd executes 'workflow list' CLI commands
var workflowListCmd = &cobra.Command{
	Use:                "list",
	Short:              "Execute 'workflow list' commands",
	Long:               `This command executes 'atmos workflow list' CLI commands`,
	Example:            "workflow list",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteWorkflowListCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(err)
		}
	},
}

func init() {
	workflowCmd.AddCommand(workflowListCmd)
}
