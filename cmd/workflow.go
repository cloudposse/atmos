package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// workflowCmd represents the base workflow command
var workflowCmd = &cobra.Command{
	Use:   "workflow [command]",
	Short: "Manage and execute Atmos workflows",
	Long: `The workflow command allows you to manage and execute Atmos workflows.

Available Commands:
  list        List all available workflows
  <name>      Execute a workflow by name

Workflows are defined in YAML files in your configured workflows directory.
Each workflow consists of a series of steps that can be executed sequentially.`,
	Example: `  # List all available workflows
  atmos workflow list

  # Execute a workflow
  atmos workflow <name> -f workflow.yaml
  atmos workflow <name> -f workflow.yaml -s <stack>
  
  # Resume a workflow from a specific step
  atmos workflow <name> -f workflow.yaml --from-step <step-name>`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			cmd.Help()
			return
		}

		for _, subcmd := range cmd.Commands() {
			if subcmd.Name() == args[0] {
				return
			}
		}

		// If we get here, it means we're trying to execute a workflow
		err := e.ExecuteWorkflowCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(schema.CliConfiguration{}, err)
		}
	},
	// custom error handling for unknown commands
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return nil // Help message will be shown
		}

		// Check if this is a known subcommand
		for _, subcmd := range cmd.Commands() {
			if subcmd.Name() == args[0] {
				return nil
			}
		}

		// If not a subcommand, validate workflow execution
		if cmd.Flags().Lookup("file").Value.String() == "" {
			return fmt.Errorf(`
Workflow file is required when executing a workflow.

Usage:
  atmos workflow %s -f <workflow-file>

To see available workflows:
  atmos workflow list

For more information:
  atmos workflow --help`, args[0])
		}

		return nil
	},
}

// workflowListCmd represents the workflow list command
var workflowListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available workflows",
	Long: `List all available workflows in your configured workflows directory.

Workflows are YAML files that define a series of steps to be executed.
The workflows directory path is configured in your atmos.yaml file.`,
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ListWorkflows(cmd, args)
		if err != nil {
			u.LogErrorAndExit(schema.CliConfiguration{}, err)
		}
	},
}

func init() {
	workflowCmd.DisableFlagParsing = false
	workflowCmd.PersistentFlags().StringP("file", "f", "", "Workflow manifest file (required for workflow execution)")
	workflowCmd.PersistentFlags().Bool("dry-run", false, "Show what would be executed without making any changes")
	workflowCmd.PersistentFlags().StringP("stack", "s", "", "Stack to use for the workflow")
	workflowCmd.PersistentFlags().String("from-step", "", "Resume workflow from a specific step")

	// Add subcommands
	workflowCmd.AddCommand(workflowListCmd)

	RootCmd.AddCommand(workflowCmd)
}
