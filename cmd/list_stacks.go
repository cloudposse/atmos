package cmd

import (
	"fmt"
	"sort"
	"strings"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/fatih/color"
	"github.com/samber/lo"
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

		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		cliConfig, err := config.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error initializing CLI config: %v", err), color.New(color.FgRed))
			return
		}

		stacksMap, err := e.ExecuteDescribeStacks(cliConfig, "", nil, nil, nil, false, false)
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error describing stacks: %v", err), color.New(color.FgRed))
			return
		}

		var output string
		if componentFlag != "" {
			// Filter stacks by component
			filteredStacks := []string{}
			for stackName, stackData := range stacksMap {
				if v2, ok := stackData.(map[string]any); ok {
					if v3, ok := v2["components"].(map[string]any); ok {
						if v4, ok := v3["terraform"].(map[string]any); ok {
							if _, exists := v4[componentFlag]; exists {
								filteredStacks = append(filteredStacks, stackName)
							}
						}
					}
				}
			}

			if len(filteredStacks) == 0 {
				output = fmt.Sprintf("No stacks found for component '%s'", componentFlag)
			} else {
				sort.Strings(filteredStacks)
				output += strings.Join(filteredStacks, "\n")
			}
		} else {
			// List all stacks
			stacks := lo.Keys(stacksMap)
			sort.Strings(stacks)
			output = strings.Join(stacks, "\n")
		}
		u.PrintMessageInColor(output, color.New(color.FgGreen))
	},
}

func init() {
	listStacksCmd.DisableFlagParsing = false
	listStacksCmd.PersistentFlags().StringP("component", "c", "", "atmos list stacks -c <component>")
	listCmd.AddCommand(listStacksCmd)
}
