package list

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/internal/tui/templates"
	"github.com/samber/lo"
)

// FilterAndListStacks filters stacks by the given component and returns the output based on the format and template
func FilterAndListStacks(stacksMap map[string]any, component string, template string) (string, error) {
	var result any

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

		// If template is provided, convert to map for JSON processing
		if template != "" {
			stacksOutput := make(map[string]interface{})
			for _, stack := range filteredStacks {
				stacksOutput[stack] = stacksMap[stack]
			}
			result = stacksOutput
		} else {
			return strings.Join(filteredStacks, "\n") + "\n", nil
		}
	} else {
		// List all stacks
		if template != "" {
			result = stacksMap
		} else {
			stacks := lo.Keys(stacksMap)
			sort.Strings(stacks)
			return strings.Join(stacks, "\n") + "\n", nil
		}
	}

	// Process template if provided
	if template != "" {
		return templates.ProcessJSONWithTemplate(result, template)
	}

	return "", fmt.Errorf("unexpected condition in FilterAndListStacks")
}
