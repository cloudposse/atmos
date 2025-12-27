package extract

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Stacks transforms stacksMap into structured stack data.
// It returns []map[string]any suitable for the renderer pipeline.
// Exposes vars from components for template access (e.g., {{ .vars.namespace }}).
func Stacks(stacksMap map[string]any) ([]map[string]any, error) {
	defer perf.Track(nil, "extract.Stacks")()

	if stacksMap == nil {
		return nil, errUtils.ErrStackNotFound
	}

	var stacks []map[string]any

	for stackName, stackData := range stacksMap {
		stack := map[string]any{
			"stack": stackName,
		}

		// Extract vars from stack data.
		if stackMap, ok := stackData.(map[string]any); ok {
			extractStackVars(stack, stackMap)
		}

		stacks = append(stacks, stack)
	}

	return stacks, nil
}

// extractStackVars extracts vars from components and exposes them for template access.
// Vars are found within components (stackMap["components"]["terraform"]["<component>"]["vars"]).
// Templates can access any var via {{ .vars.fieldname }}.
func extractStackVars(stack map[string]any, stackMap map[string]any) {
	defer perf.Track(nil, "extract.extractStackVars")()

	// Try to get vars from any component in the stack.
	// Structure: stackMap["components"]["terraform"]["<component>"]["vars"]
	vars := findVarsFromComponents(stackMap)
	if vars == nil {
		// Set empty vars map if not found.
		stack["vars"] = map[string]any{}
		return
	}

	// Expose full vars for template access (e.g., {{ .vars.namespace }}).
	stack["vars"] = vars
}

// findVarsFromComponents finds vars from the first component in the stack.
// Components structure: stackMap["components"]["<type>"]["<component>"]["vars"].
// Checks built-in component types (terraform, helmfile, packer) plus any
// additional types registered in the component registry.
func findVarsFromComponents(stackMap map[string]any) map[string]any {
	componentsMap, ok := stackMap["components"].(map[string]any)
	if !ok {
		return nil
	}

	// Get all component types to check.
	componentTypes := getComponentTypes()

	// Check each component type.
	for _, componentType := range componentTypes {
		typeComponents, ok := componentsMap[componentType].(map[string]any)
		if !ok {
			continue
		}

		// Get vars from the first component found.
		for _, componentData := range typeComponents {
			componentMap, ok := componentData.(map[string]any)
			if !ok {
				continue
			}
			if vars, ok := componentMap["vars"].(map[string]any); ok {
				return vars
			}
		}
	}

	return nil
}

// getComponentTypes returns all component types to check.
// Includes built-in types (terraform, helmfile, packer) plus any
// additional types registered in the component registry.
func getComponentTypes() []string {
	// Start with built-in component types.
	// These are the standard types defined in pkg/config/const.go.
	typeSet := map[string]struct{}{
		config.TerraformComponentType: {},
		config.HelmfileComponentType:  {},
		config.PackerComponentType:    {},
	}

	// Add any additional types from the component registry.
	// This supports custom component types registered via plugins.
	for _, t := range component.ListTypes() {
		typeSet[t] = struct{}{}
	}

	// Convert set to slice.
	types := make([]string, 0, len(typeSet))
	for t := range typeSet {
		types = append(types, t)
	}

	return types
}

// StacksForComponent extracts stacks that contain a specific component.
func StacksForComponent(componentName string, stacksMap map[string]any) ([]map[string]any, error) {
	defer perf.Track(nil, "extract.StacksForComponent")()

	if stacksMap == nil {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrStackNotFound, componentName)
	}

	// Get all component types to check.
	componentTypes := getComponentTypes()

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
		for _, componentType := range componentTypes {
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
