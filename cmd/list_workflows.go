package cmd

import (
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
	RunE: func(cmd *cobra.Command, args []string) error {
		flags := cmd.Flags()

		fileFlag, err := flags.GetString("file")
		if err != nil {
			return err
		}

		formatFlag, err := flags.GetString("format")
		if err != nil {
			return err
		}

		delimiterFlag, err := flags.GetString("delimiter")
		if err != nil {
			return err
		}

		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		// Workflows are loaded from workflow files, not stack manifests
		atmosConfig, err := config.InitCliConfig(configAndStacksInfo, false)
		if err != nil {
			return err
		}

		output, err := l.FilterAndListWorkflows(fileFlag, atmosConfig.Workflows.List, formatFlag, delimiterFlag)
		if err != nil {
			return err
		}

		u.PrintMessageInColor(output, theme.Colors.Success)
		return nil
	},
}

func init() {
	listWorkflowsCmd.PersistentFlags().StringP("file", "f", "", "Filter workflows by file (e.g., atmos list workflows -f workflow1)")
	listWorkflowsCmd.PersistentFlags().String("format", "", "Output format (table, json, csv)")
	listWorkflowsCmd.PersistentFlags().String("delimiter", "\t", "Delimiter for csv output")
	listCmd.AddCommand(listWorkflowsCmd)
}
