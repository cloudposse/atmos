package extract

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	perf "github.com/cloudposse/atmos/pkg/perf"
)

const (
	// Component metadata field names.
	metadataEnabled = "enabled"
	metadataLocked  = "locked"

	// Field names for component data.
	fieldComponent       = "component"
	fieldComponentFolder = "component_folder"
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
func buildBaseComponent(componentName, stackName, componentType string) map[string]any {
	defer perf.Track(nil, "list.extract.buildBaseComponent")()

	return map[string]any{
		fieldComponent: componentName,
		"stack":        stackName,
		"type":         componentType,
	}
}

// enrichComponentWithMetadata adds metadata fields to a component map.
func enrichComponentWithMetadata(comp map[string]any, componentData any) {
	defer perf.Track(nil, "list.extract.enrichComponentWithMetadata")()

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

	// Extract vars to top level for easy template access ({{ .vars.tenant }}).
	if vars, ok := compMap["vars"].(map[string]any); ok {
		comp["vars"] = vars
	}

	// Extract settings to top level.
	if settings, ok := compMap["settings"].(map[string]any); ok {
		comp["settings"] = settings
	}

	// Extract component_folder for column templates.
	if folder, ok := compMap[fieldComponentFolder].(string); ok {
		comp[fieldComponentFolder] = folder
	}

	// Extract terraform_component if different from component name.
	if tfComp, ok := compMap["terraform_component"].(string); ok {
		comp["terraform_component"] = tfComp
	}

	comp["data"] = compMap
}

// extractMetadataFields extracts common metadata fields to top level.
func extractMetadataFields(comp map[string]any, metadata map[string]any) {
	defer perf.Track(nil, "list.extract.extractMetadataFields")()

	enabled := getBoolWithDefault(metadata, metadataEnabled, true)
	locked := getBoolWithDefault(metadata, metadataLocked, false)

	comp[metadataEnabled] = enabled
	comp[metadataLocked] = locked
	comp["component_type"] = getStringWithDefault(metadata, "type", "real")

	// Compute status indicators for display.
	// status: Colored dot (●) for table display.
	// status_text: Semantic text ("enabled", "disabled", "locked") for JSON/CSV/YAML.
	comp["status"] = getStatusIndicator(enabled, locked)
	comp["status_text"] = getStatusText(enabled, locked)

	// Extract component_folder from metadata.component (the terraform component path).
	// This is the relative path to the component within the components directory.
	// If metadata.component is not set, fall back to the component name.
	if folder, ok := metadata[fieldComponent].(string); ok && folder != "" {
		comp[fieldComponentFolder] = folder
	} else if componentName, ok := comp[fieldComponent].(string); ok {
		comp[fieldComponentFolder] = componentName
	}
}

// setDefaultMetadataFields sets default values for metadata fields.
func setDefaultMetadataFields(comp map[string]any) {
	defer perf.Track(nil, "list.extract.setDefaultMetadataFields")()

	comp[metadataEnabled] = true
	comp[metadataLocked] = false
	comp["component_type"] = "real"

	// Default status indicators for enabled, not locked state.
	comp["status"] = getStatusIndicator(true, false)
	comp["status_text"] = getStatusText(true, false)

	// Default component_folder to component name when no metadata.component is set.
	if componentName, ok := comp[fieldComponent].(string); ok {
		comp[fieldComponentFolder] = componentName
	}
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
