package extract

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
)

const (
	// Component metadata field names.
	metadataEnabled = "enabled"
	metadataLocked  = "locked"
)

// Components transforms stacksMap into structured component data.
// Returns []map[string]any suitable for the renderer pipeline.
func Components(stacksMap map[string]any) ([]map[string]any, error) {
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
		comp := buildBaseComponent(componentName, stackName, componentType)
		enrichComponentWithMetadata(comp, componentData)
		result = append(result, comp)
	}

	return result
}

// buildBaseComponent creates the base component map with required fields.
func buildBaseComponent(componentName, stackName, componentType string) map[string]any {
	return map[string]any{
		"component": componentName,
		"stack":     stackName,
		"type":      componentType,
	}
}

// enrichComponentWithMetadata adds metadata fields to a component map.
func enrichComponentWithMetadata(comp map[string]any, componentData any) {
	compMap, ok := componentData.(map[string]any)
	if !ok {
		return
	}

	metadata, hasMetadata := compMap["metadata"].(map[string]any)
	if hasMetadata {
		comp["metadata"] = metadata
		extractMetadataFields(comp, metadata)
	} else {
		setDefaultMetadataFields(comp)
	}

	comp["data"] = compMap
}

// extractMetadataFields extracts common metadata fields to top level.
func extractMetadataFields(comp map[string]any, metadata map[string]any) {
	comp[metadataEnabled] = getBoolWithDefault(metadata, metadataEnabled, true)
	comp[metadataLocked] = getBoolWithDefault(metadata, metadataLocked, false)
	comp["component_type"] = getStringWithDefault(metadata, "type", "real")
}

// setDefaultMetadataFields sets default values for metadata fields.
func setDefaultMetadataFields(comp map[string]any) {
	comp[metadataEnabled] = true
	comp[metadataLocked] = false
	comp["component_type"] = "real"
}

// getBoolWithDefault safely extracts a bool value or returns the default.
func getBoolWithDefault(m map[string]any, key string, defaultValue bool) bool {
	if val, ok := m[key].(bool); ok {
		return val
	}
	return defaultValue
}

// getStringWithDefault safely extracts a string value or returns the default.
func getStringWithDefault(m map[string]any, key string, defaultValue string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return defaultValue
}

// ComponentsForStack extracts components for a specific stack only.
func ComponentsForStack(stackName string, stacksMap map[string]any) ([]map[string]any, error) {
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
