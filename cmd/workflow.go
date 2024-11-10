package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// workflowCmd executes a workflow
var workflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Execute a workflow",
	Long: `This command executes a workflow: atmos workflow <name> -f <file>

Atmos workflow commands support failure handling and resume functionality:

When a workflow step fails:
 - The failed step name and command will be displayed
 - A resume command will be provided to restart from the failed step

Example:
Step 'deploy-vpc' failed!
Error: Error applying plan:
1 error occurred: AWS API call failed

Command failed: terraform apply vpc -auto-approve

To resume the workflow from this step, run:
atmos workflow deploy-infra -f workflow1 --from-step deploy-vpc

For more details refer to https://atmos.tools/cli/commands/workflow/`,
	Example: "atmos workflow\n" +
		"atmos workflow <name> -f <file>\n" +
		"atmos workflow <name> -f <file> -s <stack>\n" +
		"atmos workflow <name> -f <file> --from-step <step-name>",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteWorkflowCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(schema.CliConfiguration{}, err)
		}
	},
}

func init() {
	workflowCmd.DisableFlagParsing = false
	workflowCmd.PersistentFlags().StringP("file", "f", "", "atmos workflow <name> -f <file>")
	workflowCmd.PersistentFlags().Bool("dry-run", false, "atmos workflow <name> -f <file> --dry-run")
	workflowCmd.PersistentFlags().StringP("stack", "s", "", "atmos workflow <name> -f <file> -s <stack>")
	workflowCmd.PersistentFlags().String("from-step", "", "atmos workflow <name> -f <file> --from-step <step-name>")

	RootCmd.AddCommand(workflowCmd)
}
