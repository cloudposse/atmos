package list

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
)

// ExtractStacks transforms stacksMap into structured stack data.
// Returns []map[string]any suitable for the renderer pipeline.
func ExtractStacks(stacksMap map[string]any) ([]map[string]any, error) {
	if stacksMap == nil {
		return nil, errUtils.ErrStackNotFound
	}

	var stacks []map[string]any

	for stackName := range stacksMap {
		stack := map[string]any{
			"stack": stackName,
		}

		stacks = append(stacks, stack)
	}

	return stacks, nil
}

// ExtractStacksForComponent extracts stacks that contain a specific component.
func ExtractStacksForComponent(componentName string, stacksMap map[string]any) ([]map[string]any, error) {
	if stacksMap == nil {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrStackNotFound, componentName)
	}

	var stacks []map[string]any

	for stackName, stackData := range stacksMap {
		stackMap, ok := stackData.(map[string]any)
		if !ok {
			continue // Skip invalid stacks.
		}

		componentsMap, ok := stackMap["components"].(map[string]any)
		if !ok {
			continue // Skip stacks without components.
		}

		// Check if component exists in any component type.
		found := false
		for _, componentType := range []string{"terraform", "helmfile", "packer"} {
			if typeComponents, ok := componentsMap[componentType].(map[string]any); ok {
				if _, exists := typeComponents[componentName]; exists {
					found = true
					break
				}
			}
		}

		if found {
			stack := map[string]any{
				"stack":     stackName,
				"component": componentName,
			}
			stacks = append(stacks, stack)
		}
	}

	if len(stacks) == 0 {
		return nil, errUtils.ErrNoStacksFound
	}

	return stacks, nil
}
