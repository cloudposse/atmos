package list

import (
	"fmt"
	"sort"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/fatih/color"
	"github.com/samber/lo"
)

// FilterAndListStacks filters stacks by the given component
func FilterAndListStacks(component string) ([]string, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	cliConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		u.PrintMessageInColor(fmt.Sprintf("Error initializing CLI config: %v", err), color.New(color.FgRed))
		return nil, err
	}

	stacksMap, err := e.ExecuteDescribeStacks(cliConfig, "", nil, nil, nil, false, false, false)
	if err != nil {
		u.PrintMessageInColor(fmt.Sprintf("Error describing stacks: %v", err), color.New(color.FgRed))
		return nil, err
	}

	var filteredStacks []string

	if component != "" {
		// Filter stacks by component
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
			return nil, fmt.Errorf("no stacks found for component '%s'", component)
		}
	} else {
		// List all stacks
		filteredStacks = lo.Keys(stacksMap)
	}

	// Sort the result
	sort.Strings(filteredStacks)

	return filteredStacks, nil
}
