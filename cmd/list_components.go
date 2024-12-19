package cmd

import (
	"fmt"

	e "github.com/cloudposse/atmos/internal/exec"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// listComponentsCmd lists atmos components
var listComponentsCmd = &cobra.Command{
	Use:   "components",
	Short: "Execute 'list components' command",
	Long:  `This command lists all Atmos components or filters components by stacks.`,
	Example: "atmos list components\n" +
		"atmos list components -s <stack>",
	Run: func(cmd *cobra.Command, args []string) {
		stackFlag, _ := cmd.Flags().GetString("stack")

		atmosConfig := cmd.Context().Value(contextKey("atmos_config")).(schema.AtmosConfiguration)

		stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, false, false)
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
