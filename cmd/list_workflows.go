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
		"atmos list workflows -f &ltfile&gt\n" +
		"atmos list workflows --format json\n" +
		"atmos list workflows --format csv --delimiter `,`",
	Run: func(cmd *cobra.Command, args []string) {
		flags := cmd.Flags()

		fileFlag, err := flags.GetString("file")
		if err != nil {
			u.PrintInvalidUsageErrorAndExit(fmt.Errorf("Error getting the `file` flag: %v", err), "")
			return
		}

		formatFlag, err := flags.GetString("format")
		if err != nil {
			u.PrintInvalidUsageErrorAndExit(fmt.Errorf("Error getting the `format` flag: %v", err), "")
			return
		}

		delimiterFlag, err := flags.GetString("delimiter")
		if err != nil {
			u.PrintInvalidUsageErrorAndExit(fmt.Errorf("Error getting the `delimiter` flag: %v", err), "")
			return
		}

		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			u.PrintErrorMarkdownAndExit("Error initializing CLI config", err, "")
			return
		}

		output, err := l.FilterAndListWorkflows(fileFlag, atmosConfig.Workflows.List, formatFlag, delimiterFlag)
		if err != nil {
			u.PrintErrorMarkdownAndExit("", err, "")
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
