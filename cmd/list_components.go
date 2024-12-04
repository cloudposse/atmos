package cmd

import (
	"fmt"

	l "github.com/cloudposse/atmos/pkg/list"
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
		// Check Atmos configuration
		checkAtmosConfig()

		stackFlag, _ := cmd.Flags().GetString("stack")

		componentList, err := l.FilterAndListComponents(stackFlag)
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error: %v"+"\n", err), color.New(color.FgYellow))
			return
		}

		u.PrintMessageInColor(componentList, color.New(color.FgGreen))
	},
}

func init() {
	listComponentsCmd.PersistentFlags().StringP("stack", "s", "", "Filter components by stack (e.g., atmos list components -s stack1)")
	listCmd.AddCommand(listComponentsCmd)
}
