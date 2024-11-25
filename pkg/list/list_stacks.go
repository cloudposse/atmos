package list

import (
	"fmt"
	"sort"
	"strings"

	"github.com/samber/lo"
)

// FilterAndListStacks filters stacks by the given component
func FilterAndListStacks(stacksMap map[string]any, component string) (string, error) {
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
