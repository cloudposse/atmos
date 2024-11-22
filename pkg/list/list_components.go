package list

import (
	"fmt"
	"sort"
	"strings"

	"github.com/samber/lo"
)

// FilterAndListComponents filters and lists components based on the given stack
func FilterAndListComponents(stackFlag string, stacksMap map[string]any) (string, error) {
	components := []string{}

	if stackFlag != "" {
		// Filter components for the specified stack
		if stackData, ok := stacksMap[stackFlag]; ok {
			if stackMap, ok := stackData.(map[string]any); ok {
				if componentsMap, ok := stackMap["components"].(map[string]any); ok {
					if terraformComponents, ok := componentsMap["terraform"].(map[string]any); ok {
						components = append(components, lo.Keys(terraformComponents)...)
					}
				}
			}
		} else {
			return "", fmt.Errorf("stack '%s' not found", stackFlag)
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

	// Remove duplicates and sort components
	components = lo.Uniq(components)
	sort.Strings(components)

	if len(components) == 0 {
		return "No components found", nil
	}
	return strings.Join(components, "\n") + "\n", nil
}
