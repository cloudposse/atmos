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

		components := []string{}
		if stackFlag != "" {
			if stackData, ok := stacksMap[stackFlag]; ok {
				if stackMap, ok := stackData.(map[string]any); ok {
					if componentsMap, ok := stackMap["components"].(map[string]any); ok {
						if terraformComponents, ok := componentsMap["terraform"].(map[string]any); ok {
							components = append(components, lo.Keys(terraformComponents)...)
						}
					}
				}
			} else {
				u.PrintMessageInColor(fmt.Sprintf("Stack '%s' not found", stackFlag), color.New(color.FgYellow))
				return
			}
		} else {
			// Get all components from all stacks
			for _, stackData := range stacksMap {
				if stackMap, ok := stackData.(map[string]any); ok {
					if componentsMap, ok := stackMap["components"].(map[string]any); ok {
						if terraformComponents, ok := componentsMap["terraform"].(map[string]any); ok {
							components = append(components, lo.Keys(terraformComponents)...)
						}
					}
				}
			}
		}

		components = lo.Uniq(components)
		sort.Strings(components)

		if len(components) == 0 {
			u.PrintMessageInColor("No components found", color.New(color.FgYellow))
		} else {
			u.PrintMessageInColor(strings.Join(components, "\n")+"\n", color.New(color.FgGreen))
		}
	},
}

func init() {
	listComponentsCmd.PersistentFlags().StringP("stack", "s", "", "Filter components by stack (e.g., atmos list components -s stack1)")
	listCmd.AddCommand(listComponentsCmd)
}
