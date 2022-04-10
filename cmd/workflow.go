package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// workflowCmd executes a workflow
var workflowCmd = &cobra.Command{
	Use:                "workflow",
	Short:              "Execute a workflow",
	Long:               `This command executes a workflow: atmos workflow <name> -f <file>`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteWorkflow(cmd, args)
		if err != nil {
			u.PrintErrorToStdErrorAndExit(err)
		}
	},
}

func init() {
	workflowCmd.DisableFlagParsing = false
	workflowCmd.PersistentFlags().StringP("file", "f", "", "atmos workflow <name> -f <file>")
	workflowCmd.PersistentFlags().Bool("dry-run", false, "atmos workflow <name> -f <file> --dry-run")

	err := workflowCmd.MarkPersistentFlagRequired("file")
	if err != nil {
		u.PrintErrorToStdErrorAndExit(err)
	}

	RootCmd.AddCommand(workflowCmd)
}
