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
			if v2, ok := stackData.(map[string]any); ok {
				if v3, ok := v2["components"].(map[string]any); ok {
					if v4, ok := v3["terraform"].(map[string]any); ok {
						if _, exists := v4[component]; exists {
							filteredStacks = append(filteredStacks, stackName)
						}
					}
				}
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
