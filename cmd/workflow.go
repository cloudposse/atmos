package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// workflowCmd executes a workflow
var workflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Execute a workflow",
	Long:  `This command executes a workflow: atmos workflow <name> --file <file>`,
	Example: "atmos workflow\n" +
		"atmos workflow <name> --file <file>\n" +
		"atmos workflow <name> --file <file> --stack <stack>\n" +
		"atmos workflow <name> --file <file> --from-step <step-name>\n\n" +
		"To resume the workflow from this step, run:\n" +
		"atmos workflow deploy-infra --file workflow1 --from-step deploy-vpc\n\n" +
		"For more details refer to https://atmos.tools/cli/commands/workflow/",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		// If the first arg is "list", show a clear error message
		if len(args) > 0 && args[0] == "list" {
			errorMsg := "# Error! Invalid command.\n\n" +
				"## Usage\n" +
				"`atmos workflow <name> --file <file>`\n\n" +
				"Run `atmos workflow --help` for more information."

			rendered, err := utils.RenderMarkdown(errorMsg, "dark")
			if err != nil {
				fmt.Println(errorMsg) // Fallback to plain text if rendering fails
			} else {
				fmt.Print(rendered)
			}
			return
		}

		err := e.ExecuteWorkflowCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
		}
	},
}

func init() {
	workflowCmd.DisableFlagParsing = false
	workflowCmd.PersistentFlags().StringP("file", "f", "", "atmos workflow <name> --file <file>")
	workflowCmd.PersistentFlags().Bool("dry-run", false, "atmos workflow <name> --file <file> --dry-run")
	workflowCmd.PersistentFlags().StringP("stack", "s", "", "atmos workflow <name> --file <file> --stack <stack>")
	workflowCmd.PersistentFlags().String("from-step", "", "atmos workflow <name> --file <file> --from-step <step-name>")

	RootCmd.AddCommand(workflowCmd)
}
