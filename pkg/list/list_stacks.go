package list

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
)

// FilterAndListStacks filters stacks by the given component
func FilterAndListStacks(component string) (string, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	cliConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		u.PrintMessageInColor(fmt.Sprintf("Error initializing CLI config: %v", err), color.New(color.FgRed))
	}

	stacksMap, err := e.ExecuteDescribeStacks(cliConfig, "", nil, nil, nil, false, false, false)
	if err != nil {
		u.PrintMessageInColor(fmt.Sprintf("Error describing stacks: %v", err), color.New(color.FgRed))
	}

	if component != "" {
		// Filter stacks by component
		filteredStacks := []string{}
		for stackName, stackData := range stacksMap {
			v2, ok := stackData.(map[string]any)
			if !ok {
				continue
			}
			components, ok := v2["components"].(map[string]any)
			if !ok {
				continue
			}
			terraform, ok := components["terraform"].(map[string]any)
			if !ok {
				continue
			}
			if _, exists := terraform[component]; exists {
				filteredStacks = append(filteredStacks, stackName)
			}
		}

		if len(filteredStacks) == 0 {
			return fmt.Sprintf("No stacks found for component '%s'"+"\n", component), nil
		}
		sort.Strings(filteredStacks)
		return strings.Join(filteredStacks, "\n") + "\n", nil
	}

	// List all stacks
	stacks := lo.Keys(stacksMap)
	sort.Strings(stacks)
	return strings.Join(stacks, "\n") + "\n", nil
}
