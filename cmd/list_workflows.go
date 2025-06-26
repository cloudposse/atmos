package cmd

import (
	"fmt"
	atmoserr "github.com/cloudposse/atmos/errors"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/config"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// listWorkflowsCmd lists atmos workflows
var listWorkflowsCmd = &cobra.Command{
	Use:   "workflows",
	Short: "List all Atmos workflows",
	Long:  "List Atmos workflows, with options to filter results by specific files.",
	Run: func(cmd *cobra.Command, args []string) {
		flags := cmd.Flags()

		fileFlag, err := flags.GetString("file")
		if err != nil {
			atmoserr.PrintErrorMarkdownAndExit(fmt.Errorf("Error getting the `file` flag: %v", err), "Incorrect Usage", "")
			return
		}

		formatFlag, err := flags.GetString("format")
		if err != nil {
			atmoserr.PrintErrorMarkdownAndExit(fmt.Errorf("Error getting the `format` flag: %v", err), "Incorrect Usage", "")
			return
		}

		delimiterFlag, err := flags.GetString("delimiter")
		if err != nil {
			atmoserr.PrintErrorMarkdownAndExit(fmt.Errorf("Error getting the `delimiter` flag: %v", err), "Incorrect Usage", "")
			return
		}

		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
		atmoserr.PrintErrorMarkdownAndExit(err, "Error initializing CLI config", "")

		output, err := l.FilterAndListWorkflows(fileFlag, atmosConfig.Workflows.List, formatFlag, delimiterFlag)
		atmoserr.PrintErrorMarkdownAndExit(err, "", "")

		u.PrintMessageInColor(output, theme.Colors.Success)
	},
}

func init() {
	listWorkflowsCmd.PersistentFlags().StringP("file", "f", "", "Filter workflows by file (e.g., atmos list workflows -f workflow1)")
	listWorkflowsCmd.PersistentFlags().String("format", "", "Output format (table, json, csv)")
	listWorkflowsCmd.PersistentFlags().String("delimiter", "\t", "Delimiter for csv output")
	listCmd.AddCommand(listWorkflowsCmd)
}
