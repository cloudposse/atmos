package workflow

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
)

var workflowParser *flags.StandardParser

// WorkflowOptions contains parsed flags for the workflow command.
type WorkflowOptions struct {
	global.Flags        // Embedded global flags
	File         string // Workflow file to run (optional if name is unique)
	DryRun       bool   // Simulate without making changes
	Stack        string // Stack name
	FromStep     string // Resume from specific step
	Identity     string // Identity for workflow steps
}

// workflowCmd executes a workflow.
var workflowCmd = &cobra.Command{
	Use:   "workflow [name]",
	Short: "Run predefined tasks using workflows",
	Long: `Run predefined workflows as an alternative to traditional task runners.
Workflows enable you to automate and manage infrastructure and operational tasks
specified in configuration files.

If no workflow name is provided, an interactive TUI will be displayed.
If a workflow name is provided without --file, Atmos will auto-discover the workflow
from all available workflow files. If multiple files contain a workflow with the same
name, an interactive selector will prompt you to choose which one to run.`,

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE: func(cmd *cobra.Command, args []string) error {
		handleHelpRequest(cmd, args)

		// If no arguments are provided, start the workflow UI.
		if len(args) == 0 {
			err := e.ExecuteWorkflowCmd(cmd, args)
			if err != nil {
				return err
			}
			// Return after TUI execution to prevent showing usage error.
			return nil
		}

		// Parse flags into options struct.
		v := viper.GetViper()
		if err := workflowParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := &WorkflowOptions{
			Flags:    flags.ParseGlobalFlags(cmd, v),
			File:     v.GetString("file"),
			DryRun:   v.GetBool("dry-run"),
			Stack:    v.GetString("stack"),
			FromStep: v.GetString("from-step"),
			Identity: v.GetString("identity"),
		}

		// Execute the workflow command with options.
		return executeWorkflowWithOptions(cmd, args, opts)
	},
}

func init() {
	// Create StandardParser with functional options.
	workflowParser = flags.NewStandardParser(
		flags.WithStringFlag("file", "f", "", "Specify workflow file to run (optional if workflow name is unique)"),
		flags.WithBoolFlag("dry-run", "", false, "Simulate the workflow without making any changes"),
		flags.WithStringFlag("stack", "s", "", "Stack name"),
		flags.WithStringFlag("from-step", "", "", "Resume the workflow from the specified step"),
		flags.WithStringFlag("identity", "", "", "Identity to use for workflow steps that don't specify their own identity"),
		flags.WithEnvVars("file", "ATMOS_WORKFLOW_FILE"),
		flags.WithEnvVars("dry-run", "ATMOS_WORKFLOW_DRY_RUN"),
		flags.WithEnvVars("stack", "ATMOS_STACK"),
		flags.WithEnvVars("from-step", "ATMOS_WORKFLOW_FROM_STEP"),
		flags.WithEnvVars("identity", "ATMOS_IDENTITY"),
	)

	// Register flags on command.
	workflowParser.RegisterFlags(workflowCmd)

	// Bind to Viper.
	if err := workflowParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Add shell completions.
	addStackCompletion(workflowCmd)
	addIdentityCompletion(workflowCmd)

	// Register workflow name completion for the first positional argument.
	workflowCmd.ValidArgsFunction = workflowNameCompletion

	// Register with command registry.
	internal.Register(&WorkflowCommandProvider{})
}

// WorkflowCommandProvider implements the CommandProvider interface.
type WorkflowCommandProvider struct{}

// GetCommand returns the workflow command.
func (w *WorkflowCommandProvider) GetCommand() *cobra.Command {
	return workflowCmd
}

// GetName returns the command name.
func (w *WorkflowCommandProvider) GetName() string {
	return "workflow"
}

// GetGroup returns the command group.
func (w *WorkflowCommandProvider) GetGroup() string {
	return "Core Stack Commands"
}

// executeWorkflowWithOptions executes the workflow command with parsed options.
func executeWorkflowWithOptions(cmd *cobra.Command, args []string, _ *WorkflowOptions) error {
	// Execute the workflow command and return any errors to main.go for centralized formatting.
	return e.ExecuteWorkflowCmd(cmd, args)
}

// handleHelpRequest handles the help request for the workflow command.
func handleHelpRequest(cmd *cobra.Command, args []string) {
	// Check if help is requested.
	if len(args) > 0 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h") {
		_ = cmd.Help()
	}
}
