package cmd

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// listComponentsCmd lists atmos components
var listComponentsCmd = &cobra.Command{
	Use:   "components",
	Short: "List all Atmos components or filter by stack",
	Long:  "List Atmos components, with options to filter results by specific stacks.",
	Example: "atmos list components\n" +
		"atmos list components -s <stack>",
	Run: func(cmd *cobra.Command, args []string) {
		handleHelpRequest(cmd, args, false)
		if hasPositionalArgs(args) {
			showUsageAndExit(cmd, args, false)
		}
		// Check Atmos configuration
		checkAtmosConfig()

		flags := cmd.Flags()

		stackFlag, err := flags.GetString("stack")
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error getting the 'stack' flag: %v", err), color.New(color.FgRed))
			return
		}

		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error initializing CLI config: %v", err), theme.Colors.Error)
			return
		}

		stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, false, false)
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error describing stacks: %v", err), theme.Colors.Error)
			return
		}

		output, err := l.FilterAndListComponents(stackFlag, stacksMap)
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error: %v"+"\n", err), theme.Colors.Warning)
			return
		}

		u.PrintMessageInColor(output, theme.Colors.Success)
	},
}

func init() {
	listComponentsCmd.PersistentFlags().StringP("stack", "s", "", "Filter components by stack (e.g., atmos list components -s stack1)")
	listCmd.AddCommand(listComponentsCmd)
}
