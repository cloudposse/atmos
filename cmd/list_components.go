package cmd

import (
	"fmt"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// listComponentsCmd lists atmos components
var listComponentsCmd = &cobra.Command{
	Use:   "components",
	Short: "List all Atmos components or filter by stack",
	Long:  "This command lists all Atmos components, with the option to filter by specific stacks.",
	Example: "atmos list components\n" +
		"atmos list components -s <stack>",
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		stackFlag, _ := cmd.Flags().GetString("stack")

		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		cliConfig, err := config.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error initializing CLI config: %v", err), color.New(color.FgRed))
			return
		}

		stacksMap, err := e.ExecuteDescribeStacks(cliConfig, "", nil, nil, nil, false, false, false)
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error describing stacks: %v", err), color.New(color.FgRed))
			return
		}

		output, err := l.FilterAndListComponents(stackFlag, stacksMap)
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error: %v"+"\n", err), color.New(color.FgYellow))
			return
		}

		u.PrintMessageInColor(output, color.New(color.FgGreen))
	},
}

func init() {
	listComponentsCmd.PersistentFlags().StringP("stack", "s", "", "Filter components by stack (e.g., atmos list components -s stack1)")
	listCmd.AddCommand(listComponentsCmd)
}
