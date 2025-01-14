package cmd

import (
	"fmt"

	"github.com/fatih/color"
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
	Long:  "List Atmos workflows, showing their associated files and workflow names for easy reference.",
	Example: "atmos list workflows\n" +
		"atmos list workflows -f <file>",
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		flags := cmd.Flags()

		fileFlag, err := flags.GetString("file")
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error getting the 'file' flag: %v", err), color.New(color.FgRed))
			return
		}

		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error initializing CLI config: %v", err), theme.Colors.Error)
			return
		}

		output, err := l.FilterAndListWorkflows(fileFlag, atmosConfig.Workflows.List)
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error: %v"+"\n", err), theme.Colors.Warning)
			return
		}

		u.PrintMessageInColor(output, theme.Colors.Success)
	},
}

func init() {
	listWorkflowsCmd.PersistentFlags().StringP("file", "f", "", "Filter workflows by file (e.g., atmos list workflows -f workflow1)")
	listCmd.AddCommand(listWorkflowsCmd)
}
