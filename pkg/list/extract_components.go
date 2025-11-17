package list

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
)

// ComponentData represents a component with all its attributes for listing.
type ComponentData struct {
	Component string
	Stack     string
	Type      string // terraform, helmfile, packer, etc.
	Enabled   bool
	Locked    bool
	Metadata  map[string]any
}

// ExtractComponents transforms stacksMap into structured component data.
// Returns []map[string]any suitable for the renderer pipeline.
func ExtractComponents(stacksMap map[string]any) ([]map[string]any, error) {
	if stacksMap == nil {
		return nil, errUtils.ErrStackNotFound
	}

	var components []map[string]any

	for stackName, stackData := range stacksMap {
		stackMap, ok := stackData.(map[string]any)
		if !ok {
			continue // Skip invalid stacks.
		}

		componentsMap, ok := stackMap["components"].(map[string]any)
		if !ok {
			continue // Skip stacks without components.
		}

		// Process each component type.
		components = append(components, extractComponentType(stackName, "terraform", componentsMap)...)
		components = append(components, extractComponentType(stackName, "helmfile", componentsMap)...)
		components = append(components, extractComponentType(stackName, "packer", componentsMap)...)

		// TODO: Add support for plugin component types from schema.Components.Plugins
	}

	return components, nil
}

// extractComponentType extracts components of a specific type from a stack.
func extractComponentType(stackName, componentType string, componentsMap map[string]any) []map[string]any {
	typeComponents, ok := componentsMap[componentType].(map[string]any)
	if !ok {
		return nil
	}

	var result []map[string]any
	for componentName, componentData := range typeComponents {
		comp := map[string]any{
			"component": componentName,
			"stack":     stackName,
			"type":      componentType,
		}

		// Extract metadata if available.
		if compMap, ok := componentData.(map[string]any); ok {
			// Extract metadata fields.
			if metadata, ok := compMap["metadata"].(map[string]any); ok {
				comp["metadata"] = metadata

				// Extract common metadata fields to top level for easy filtering.
				if enabled, ok := metadata["enabled"].(bool); ok {
					comp["enabled"] = enabled
				} else {
					comp["enabled"] = true // Default to enabled.
				}

				if locked, ok := metadata["locked"].(bool); ok {
					comp["locked"] = locked
				} else {
					comp["locked"] = false // Default to unlocked.
				}

				if compType, ok := metadata["type"].(string); ok {
					comp["component_type"] = compType // real/abstract.
				} else {
					comp["component_type"] = "real" // Default to real.
				}
			} else {
				// No metadata - use defaults.
				comp["enabled"] = true
				comp["locked"] = false
				comp["component_type"] = "real"
			}

			// Store full component data for template access.
			comp["data"] = compMap
		}

		result = append(result, comp)
	}

	return result
}

// ExtractComponentsForStack extracts components for a specific stack only.
func ExtractComponentsForStack(stackName string, stacksMap map[string]any) ([]map[string]any, error) {
	stackData, ok := stacksMap[stackName]
	if !ok {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrStackNotFound, stackName)
	}

	stackMap, ok := stackData.(map[string]any)
	if !ok {
		return nil, errUtils.ErrParseStacks
	}

	componentsMap, ok := stackMap["components"].(map[string]any)
	if !ok {
		return nil, errUtils.ErrParseComponents
	}

	var components []map[string]any
	components = append(components, extractComponentType(stackName, "terraform", componentsMap)...)
	components = append(components, extractComponentType(stackName, "helmfile", componentsMap)...)
	components = append(components, extractComponentType(stackName, "packer", componentsMap)...)

	if len(components) == 0 {
		return nil, errUtils.ErrNoComponentsFound
	}

	return components, nil
}
