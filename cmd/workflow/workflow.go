package workflow

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

var workflowParser *flags.StandardParser

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
		// Handle "atmos workflow help" - show help and return.
		// Note: Cobra already handles -h/--help flags before RunE executes.
		if len(args) > 0 && args[0] == "help" {
			return cmd.Help()
		}

		// If no arguments are provided, start the workflow UI.
		if len(args) == 0 {
			err := e.ExecuteWorkflowCmd(cmd, args)
			if err != nil {
				return err
			}
			// Return after TUI execution to prevent showing usage error.
			return nil
		}

		// Execute the workflow command.
		return e.ExecuteWorkflowCmd(cmd, args)
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

// GetFlagsBuilder returns the flags builder for this command.
func (w *WorkflowCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

// GetPositionalArgsBuilder returns the positional args builder for this command.
func (w *WorkflowCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityFlags returns compatibility flags for this command.
func (w *WorkflowCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// GetAliases returns command aliases (none for workflow).
func (w *WorkflowCommandProvider) GetAliases() []internal.CommandAlias {
	return nil
}

// IsExperimental returns whether this command is experimental.
func (w *WorkflowCommandProvider) IsExperimental() bool {
	return false
}
