package cmd

import (
	"fmt"

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
	Example: "atmos list workflows\n" +
		"atmos list workflows -f <file>\n" +
		"atmos list workflows --format json\n" +
		"atmos list workflows --format csv --delimiter ','",
	Run: func(cmd *cobra.Command, args []string) {
		flags := cmd.Flags()

		fileFlag, err := flags.GetString("file")
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error getting the 'file' flag: %v", err), theme.Colors.Error)
			return
		}

		formatFlag, err := flags.GetString("format")
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error getting the 'format' flag: %v", err), theme.Colors.Error)
			return
		}

		delimiterFlag, err := flags.GetString("delimiter")
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error getting the 'delimiter' flag: %v", err), theme.Colors.Error)
			return
		}

		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error initializing CLI config: %v", err), theme.Colors.Error)
			return
		}

		output, err := l.FilterAndListWorkflows(fileFlag, atmosConfig.Workflows.List, formatFlag, delimiterFlag)
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error: %v"+"\n", err), theme.Colors.Warning)
			return
		}

		u.PrintMessageInColor(output, theme.Colors.Success)
	},
}

func init() {
	listWorkflowsCmd.PersistentFlags().StringP("file", "f", "", "Filter workflows by file (e.g., atmos list workflows -f workflow1)")
	listWorkflowsCmd.PersistentFlags().String("format", "", "Output format (table, json, csv)")
	listWorkflowsCmd.PersistentFlags().String("delimiter", "\t", "Delimiter for csv output")
	listCmd.AddCommand(listWorkflowsCmd)
}
