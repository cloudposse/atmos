package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
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

		output, err := l.FilterAndListStacks(stacksMap, componentFlag)
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error filtering stacks: %v", err), theme.Colors.Error)
			return
		}
		u.PrintMessageInColor(output, theme.Colors.Success)
	},
}

func init() {
	listStacksCmd.DisableFlagParsing = false
	listStacksCmd.PersistentFlags().StringP("component", "c", "", "atmos list stacks -c <component>")
	listCmd.AddCommand(listStacksCmd)
}
