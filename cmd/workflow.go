package cmd

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
)

//go:embed markdown/workflow.md
var workflowMarkdown string

// workflowCmd executes a workflow
var workflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Run predefined tasks using workflows",
	Long:  `Run predefined workflows as an alternative to traditional task runners. Workflows enable you to automate and manage infrastructure and operational tasks specified in configuration files.`,

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
	workflowCmd.PersistentFlags().StringP("file", "f", "", "Specify the workflow file to run")
	workflowCmd.PersistentFlags().Bool("dry-run", false, "Simulate the workflow without making any changes")
	AddStackCompletion(workflowCmd)
	workflowCmd.PersistentFlags().String("from-step", "", "Resume the workflow from the specified step")
	workflowCommandConfig()
	RootCmd.AddCommand(workflowCmd)
}

func workflowCommandConfig() {
	config.DefaultConfigHandler.AddConfig(workflowCmd, &config.ConfigOptions{
		FlagName:     "workflows-dir",
		EnvVar:       "ATMOS_WORKFLOWS_BASE_PATH",
		Description:  "Base path for workflows configurations.",
		Key:          "workflows.base_path",
		DefaultValue: "",
	})
	config.DefaultConfigHandler.AddConfig(workflowCmd, &config.ConfigOptions{
		FlagName:     "workflows-base-path",
		EnvVar:       "ATMOS_WORKFLOWS_BASE_PATH",
		Description:  "Base path for workflows configurations.",
		Key:          "workflows.base_path",
		DefaultValue: "",
	})
}
