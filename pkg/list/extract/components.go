package extract

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	perf "github.com/cloudposse/atmos/pkg/perf"
)

// computeComponentStatus computes status and status_text for a component.
// It uses the already-extracted enabled/locked values from the component map.
func computeComponentStatus(comp map[string]any) {
	enabled := true
	locked := false

	if val, ok := comp[metadataEnabled].(bool); ok {
		enabled = val
	}
	if val, ok := comp[metadataLocked].(bool); ok {
		locked = val
	}

	comp["status"] = getStatusIndicator(enabled, locked)
	comp["status_text"] = getStatusText(enabled, locked)
}

const (
	// Component metadata field names.
	metadataEnabled = "enabled"
	metadataLocked  = "locked"
)

// Components transforms stacksMap into structured component data.
// Returns []map[string]any suitable for the renderer pipeline.
func Components(stacksMap map[string]any) ([]map[string]any, error) {
	defer perf.Track(nil, "list.extract.Components")()

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
	defer perf.Track(nil, "list.extract.extractComponentType")()

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
func buildBaseComponent(componentName, stackName, componentKind string) map[string]any {
	defer perf.Track(nil, "list.extract.buildBaseComponent")()

	return map[string]any{
		"component": componentName,
		"stack":     stackName,
		"kind":      componentKind, // terraform, helmfile, packer
	}
}

// enrichComponentWithMetadata adds metadata fields to a component map.
func enrichComponentWithMetadata(comp map[string]any, componentData any) {
	defer perf.Track(nil, "list.extract.enrichComponentWithMetadata")()

	compMap, ok := componentData.(map[string]any)
	if !ok {
		return
	}

	// Extract component_path from component_info if available.
	if componentInfo, ok := compMap["component_info"].(map[string]any); ok {
		if path, ok := componentInfo["component_path"].(string); ok {
			comp["component_path"] = path
		}
	}

	metadata, hasMetadata := compMap["metadata"].(map[string]any)
	if hasMetadata {
		comp["metadata"] = metadata
		extractMetadataFields(comp, metadata)
	} else {
		setDefaultMetadataFields(comp)
	}

	// Compute status indicator after enabled/locked are set.
	computeComponentStatus(comp)

	comp["data"] = compMap
}

// extractMetadataFields extracts common metadata fields to top level.
func extractMetadataFields(comp map[string]any, metadata map[string]any) {
	defer perf.Track(nil, "list.extract.extractMetadataFields")()

	comp[metadataEnabled] = getBoolWithDefault(metadata, metadataEnabled, true)
	comp[metadataLocked] = getBoolWithDefault(metadata, metadataLocked, false)
	comp["type"] = getStringWithDefault(metadata, "type", "real") // real, abstract
}

// setDefaultMetadataFields sets default values for metadata fields.
func setDefaultMetadataFields(comp map[string]any) {
	defer perf.Track(nil, "list.extract.setDefaultMetadataFields")()

	comp[metadataEnabled] = true
	comp[metadataLocked] = false
	comp["type"] = "real" // real, abstract
}

// getBoolWithDefault safely extracts a bool value or returns the default.
func getBoolWithDefault(m map[string]any, key string, defaultValue bool) bool {
	defer perf.Track(nil, "list.extract.getBoolWithDefault")()

	if val, ok := m[key].(bool); ok {
		return val
	}
	return defaultValue
}

// getStringWithDefault safely extracts a string value or returns the default.
func getStringWithDefault(m map[string]any, key string, defaultValue string) string {
	defer perf.Track(nil, "list.extract.getStringWithDefault")()

	if val, ok := m[key].(string); ok {
		return val
	}
	return defaultValue
}

// ComponentsForStack extracts components for a specific stack only.
func ComponentsForStack(stackName string, stacksMap map[string]any) ([]map[string]any, error) {
	defer perf.Track(nil, "list.extract.ComponentsForStack")()

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
