package cmd

import (
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
)

// workflowCmd executes a workflow
var workflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Run predefined tasks using workflows",
	Long:  `Run predefined workflows as an alternative to traditional task runners. Workflows enable you to automate and manage infrastructure and operational tasks specified in configuration files.`,

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE: func(cmd *cobra.Command, args []string) error {
		handleHelpRequest(cmd, args)
		// If no arguments are provided, start the workflow UI
		if len(args) == 0 {
			err := e.ExecuteWorkflowCmd(cmd, args)
			if err != nil {
				return err
			}
		}

		// Get the --file flag value
		workflowFile, _ := cmd.Flags().GetString("file")

		// If no file is provided, show the usage information
		if workflowFile == "" {
			err := cmd.Usage()
			if err != nil {
				return err
			}
		}

		// Execute the workflow command
		err := e.ExecuteWorkflowCmd(cmd, args)
		if err != nil {
			// Check if it's a known error that's already printed in ExecuteWorkflowCmd.
			// If it is, we don't need to print it again, but we do need to exit with the proper exit code.
			if e.IsKnownWorkflowError(err) {
				exitCode := errUtils.GetExitCode(err)
				errUtils.Exit(exitCode)
			}
			return err
		}

		return nil
	},
}

func init() {
	workflowCmd.DisableFlagParsing = false
	workflowCmd.PersistentFlags().StringP("file", "f", "", "Specify the workflow file to run")
	workflowCmd.PersistentFlags().Bool("dry-run", false, "Simulate the workflow without making any changes")
	AddStackCompletion(workflowCmd)
	workflowCmd.PersistentFlags().String("from-step", "", "Resume the workflow from the specified step")

	RootCmd.AddCommand(workflowCmd)
}
