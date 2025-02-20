package list

import (
	"fmt"
	"sort"

	"github.com/samber/lo"
)

// getStackComponents extracts Terraform components from the final map of stacks.
func getStackComponents(stackData any) ([]string, error) {
	stackMap, ok := stackData.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("could not parse stacks")
	}

	componentsMap, ok := stackMap["components"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("could not parse components")
	}

	terraformComponents, ok := componentsMap["terraform"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("could not parse Terraform components")
	}

	return lo.Keys(terraformComponents), nil
}

// FilterAndListComponents filters and lists components based on the given stack.
func FilterAndListComponents(stackFlag string, stacksMap map[string]any) ([]string, error) {
	components := []string{}

	if stackFlag != "" {
		// Filter components for the specified stack
		if stackData, ok := stacksMap[stackFlag]; ok {
			stackComponents, err := getStackComponents(stackData)
			if err != nil {
				return nil, fmt.Errorf("error processing stack '%s': %w", stackFlag, err)
			}

			components = append(components, stackComponents...)
		} else {
			return nil, fmt.Errorf("stack '%s' not found", stackFlag)
		}
	} else {
		// Get all components from all stacks
		for _, stackData := range stacksMap {
			stackComponents, err := getStackComponents(stackData)
			if err != nil {
				continue // Skip invalid stacks
			}

			components = append(components, stackComponents...)
		}
	}

	// Remove duplicates and sort components
	components = lo.Uniq(components)
	sort.Strings(components)

	if len(components) == 0 {
		return []string{}, nil
	}

	return components, nil
}
