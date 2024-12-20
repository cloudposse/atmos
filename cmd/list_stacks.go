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

// listStacksCmd lists atmos stacks
var listStacksCmd = &cobra.Command{
	Use:   "stacks",
	Short: "List all Atmos stacks or stacks for a specific component",
	Long:  "This command lists all Atmos stacks, or filters the list to show only the stacks associated with a specified component.",
	Example: "atmos list stacks\n" +
		"atmos list stacks -c <component>",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		componentFlag, _ := cmd.Flags().GetString("component")

		atmosConfig := cmd.Context().Value(contextKey("atmos_config")).(schema.AtmosConfiguration)

		stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, false, false)
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error describing stacks: %v", err), color.New(color.FgRed))
			return
		}

		output, err := l.FilterAndListStacks(stacksMap, componentFlag)
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error filtering stacks: %v", err), color.New(color.FgRed))
			return
		}
		u.PrintMessageInColor(output, color.New(color.FgGreen))
	},
}

func init() {
	listStacksCmd.DisableFlagParsing = false
	listStacksCmd.PersistentFlags().StringP("component", "c", "", "atmos list stacks -c <component>")
	listCmd.AddCommand(listStacksCmd)
}
