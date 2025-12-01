package list

import (
	"fmt"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/samber/lo"
)

func getStackComponents(stackData any) ([]string, error) {
	stackMap, ok := stackData.(map[string]any)
	if !ok {
		return nil, errUtils.ErrParseStacks
	}

	componentsMap, ok := stackMap["components"].(map[string]any)
	if !ok {
		return nil, errUtils.ErrParseComponents
	}

	var allComponents []string

	// Extract terraform components
	if terraformComponents, ok := componentsMap["terraform"].(map[string]any); ok {
		allComponents = append(allComponents, lo.Keys(terraformComponents)...)
	}

	// Extract helmfile components
	if helmfileComponents, ok := componentsMap["helmfile"].(map[string]any); ok {
		allComponents = append(allComponents, lo.Keys(helmfileComponents)...)
	}

	// Extract packer components
	if packerComponents, ok := componentsMap["packer"].(map[string]any); ok {
		allComponents = append(allComponents, lo.Keys(packerComponents)...)
	}

	// If no components found, return an error
	if len(allComponents) == 0 {
		return nil, errUtils.ErrNoComponentsFound
	}

	return allComponents, nil
}

// getComponentsForSpecificStack extracts components from a specific stack.
func getComponentsForSpecificStack(stackName string, stacksMap map[string]any) ([]string, error) {
	// Verify stack exists.
	stackData, ok := stacksMap[stackName]
	if !ok {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrStackNotFound, stackName)
	}

	// Get components for the specific stack.
	stackComponents, err := getStackComponents(stackData)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", errUtils.ErrProcessStack, stackName, err)
	}

	return stackComponents, nil
}

// processAllStacks collects components from all valid stacks.
func processAllStacks(stacksMap map[string]any) []string {
	var components []string
	for _, stackData := range stacksMap {
		stackComponents, err := getStackComponents(stackData)
		if err != nil {
			continue // Skip invalid stacks.
		}
		components = append(components, stackComponents...)
	}
	return components
}

// FilterAndListComponents filters and lists components based on the given stack.
func FilterAndListComponents(stackFlag string, stacksMap map[string]any) ([]string, error) {
	var components []string
	if stacksMap == nil {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrStackNotFound, stackFlag)
	}

	// Handle specific stack case.
	if stackFlag != "" {
		stackComponents, err := getComponentsForSpecificStack(stackFlag, stacksMap)
		if err != nil {
			return nil, err
		}
		components = stackComponents
	} else {
		// Process all stacks.
		components = processAllStacks(stacksMap)
	}

	// Remove duplicates and sort components.
	components = lo.Uniq(components)
	sort.Strings(components)

	if len(components) == 0 {
		return []string{}, nil
	}
	return components, nil
}
