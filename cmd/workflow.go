package cmd

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

//go:embed markdown/workflow.md
var workflowMarkdown string

// workflowCmd executes a workflow
var workflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Run predefined tasks using workflows",
	Long:  `Run predefined workflows as an alternative to traditional task runners. Workflows enable you to automate and manage infrastructure and operational tasks specified in configuration files.`,
	Example: "atmos workflow\n" +
		"atmos workflow &ltname&gt --file &ltfile&gt\n" +
		"atmos workflow &ltname&gt --file &ltfile&gt --stack &ltstack&gt\n" +
		"atmos workflow &ltname&gt --file &ltfile&gt --from-step &ltstep-name&gt\n\n" +
		"To resume the workflow from this step, run:\n" +
		"atmos workflow deploy-infra --file workflow1 --from-step deploy-vpc\n\n" +
		"For more details refer to https://atmos.tools/cli/commands/workflow/",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		handleHelpRequest(cmd, args)
		// If no arguments are provided, start the workflow UI
		if len(args) == 0 {
			err := e.ExecuteWorkflowCmd(cmd, args)
			if err != nil {
				u.LogErrorAndExit(err)
			}
			return
		}

		// Get the --file flag value
		workflowFile, _ := cmd.Flags().GetString("file")

		// If no file is provided, show invalid command error with usage information
		if workflowFile == "" {
			cmd.Usage()
		}

		// Execute the workflow command
		err := e.ExecuteWorkflowCmd(cmd, args)
		if err != nil {
			// Format common error messages
			if strings.Contains(err.Error(), "does not exist") {
				u.PrintErrorMarkdownAndExit("File Not Found", fmt.Errorf("`%v` was not found", workflowFile), "")
			} else if strings.Contains(err.Error(), "No workflow exists with the name") {
				u.PrintErrorMarkdownAndExit("Invalid Workflow Name", err, "")
			} else {
				// For other errors, use the standard error handler
				u.PrintErrorMarkdownAndExit("", err, "")
			}
		}
	},
}

func init() {
	workflowCmd.DisableFlagParsing = false
	workflowCmd.PersistentFlags().StringP("file", "f", "", "atmos workflow &ltname&gt --file &ltfile&gt")
	workflowCmd.PersistentFlags().Bool("dry-run", false, "atmos workflow &ltname&gt --file &ltfile&gt --dry-run")
	AddStackCompletion(workflowCmd)
	workflowCmd.PersistentFlags().String("from-step", "", "atmos workflow &ltname&gt --file &ltfile&gt --from-step &ltstep-name&gt")

	RootCmd.AddCommand(workflowCmd)
}
