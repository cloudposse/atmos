package cmd

import (
	"fmt"

	l "github.com/cloudposse/atmos/pkg/list"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// listStacksCmd lists atmos stacks
var listStacksCmd = &cobra.Command{
	Use:   "stacks",
	Short: "Execute 'list stacks' command",
	Long:  `This command lists all Atmos stacks or all stacks for the specified component: atmos list stacks -c <component>`,
	Example: "atmos list stacks\n" +
		"atmos list stacks -c <component>",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		componentFlag, _ := cmd.Flags().GetString("component")

		stackList, err := l.FilterAndListStacks(componentFlag)
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error filtering stacks: %v", err), color.New(color.FgRed))
			return
		}

		if len(stackList) == 0 {
			u.PrintMessageInColor("No stacks found.", color.New(color.FgYellow))
		} else {
			for _, stack := range stackList {
				u.PrintMessageInColor(stack+"\n", color.New(color.FgGreen))
			}
		}
	},
}

func init() {
	listStacksCmd.DisableFlagParsing = false
	listStacksCmd.PersistentFlags().StringP("component", "c", "", "atmos list stacks -c <component>")
	listCmd.AddCommand(listStacksCmd)
}
