package cmd

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags/workflow"
)

// workflowParser is created once at package initialization using builder pattern.
var workflowParser = workflow.NewWorkflowOptionsBuilder().
	WithFile(false).  // Optional file flag → .File field
	WithDryRun().     // Dry-run flag → .DryRun field
	WithFromStep().   // From-step flag → .FromStep field
	WithStack(false). // Optional stack flag → .Stack field
	WithIdentity().   // Identity flag → .Identity field
	Build()

// workflowCmd executes a workflow.
var workflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Run predefined tasks using workflows",
	Long:  `Run predefined workflows as an alternative to traditional task runners. Workflows enable you to automate and manage infrastructure and operational tasks specified in configuration files.`,

	RunE: func(cmd *cobra.Command, args []string) error {
		handleHelpRequest(cmd, args)

		// Execute the workflow command.
		// NOTE: Parser is registered and bound to Viper for precedence,
		// but ExecuteWorkflowCmd still parses flags internally.
		// Will migrate to use workflowParser.Parse() once ExecuteWorkflowCmd is refactored.
		err := e.ExecuteWorkflowCmd(cmd, args)
		if err != nil {
			// Check if it's a known error that's already printed in ExecuteWorkflowCmd.
			// If it is, we don't need to print it again, but we do need to exit with a non-zero exit code.
			if e.IsKnownWorkflowError(err) {
				// Check if the error wraps an ExitCodeError to preserve the actual exit code.
				var exitCodeErr errUtils.ExitCodeError
				if errors.As(err, &exitCodeErr) {
					errUtils.Exit(exitCodeErr.Code)
				}
				errUtils.Exit(1)
			}
			return err
		}

		return nil
	},
}

func init() {
	// Register flags to the command (automatically sets DisableFlagParsing=true).
	workflowParser.RegisterFlags(workflowCmd)
	_ = workflowParser.BindToViper(viper.GetViper())

	AddStackCompletion(workflowCmd)
	AddIdentityCompletion(workflowCmd)

	RootCmd.AddCommand(workflowCmd)
}
