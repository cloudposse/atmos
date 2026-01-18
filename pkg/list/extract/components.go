package extract

import (
	"fmt"
	"path"
	"path/filepath"

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

	// Field names for component data.
	fieldComponent       = "component"
	fieldComponentFolder = "component_folder"
	fieldMetadata        = "metadata"
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
		fieldComponent: componentName,
		"stack":        stackName,
		"type":         componentKind, // terraform, helmfile, packer
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

	metadata, hasMetadata := compMap[fieldMetadata].(map[string]any)
	if hasMetadata {
		comp[fieldMetadata] = metadata
		extractMetadataFields(comp, metadata)
	} else {
		setDefaultMetadataFields(comp)
	}

	// Compute status indicator after enabled/locked are set.
	computeComponentStatus(comp)

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

	comp[metadataEnabled] = getBoolWithDefault(metadata, metadataEnabled, true)
	comp[metadataLocked] = getBoolWithDefault(metadata, metadataLocked, false)
	comp["component_type"] = getStringWithDefault(metadata, "type", "real") // real, abstract

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
	comp["component_type"] = "real" // real, abstract

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

// UniqueComponents extracts deduplicated components from all stacks.
// Returns unique component names with aggregated metadata (stack count, types).
// This is the original behavior of "list components" - showing unique component definitions.
// The stackPattern parameter is an optional glob pattern to filter which stacks to consider.
func UniqueComponents(stacksMap map[string]any, stackPattern string) ([]map[string]any, error) {
	defer perf.Track(nil, "list.extract.UniqueComponents")()

	if stacksMap == nil {
		return nil, errUtils.ErrStackNotFound
	}

	// Use a map to deduplicate by component name + type combination.
	// Key: "componentName:componentType" (e.g., "vpc:terraform").
	seen := make(map[string]map[string]any)

	for stackName, stackData := range stacksMap {
		// Apply stack filter if provided.
		if stackPattern != "" {
			// Stack names are slash-separated; normalize for cross-platform matching.
			// Use path.Match (not filepath.Match) to ensure consistent behavior on Windows.
			name := filepath.ToSlash(stackName)
			pattern := filepath.ToSlash(stackPattern)
			matched, err := path.Match(pattern, name)
			if err != nil || !matched {
				continue
			}
		}

		stackMap, ok := stackData.(map[string]any)
		if !ok {
			continue
		}

		componentsMap, ok := stackMap["components"].(map[string]any)
		if !ok {
			continue
		}

		// Process each component type.
		extractUniqueComponentType("terraform", componentsMap, seen)
		extractUniqueComponentType("helmfile", componentsMap, seen)
		extractUniqueComponentType("packer", componentsMap, seen)
	}

	// Convert map to slice.
	var components []map[string]any
	for _, comp := range seen {
		components = append(components, comp)
	}

	return components, nil
}

// extractUniqueComponentType extracts unique components of a specific type.
func extractUniqueComponentType(componentType string, componentsMap map[string]any, seen map[string]map[string]any) {
	typeComponents, ok := componentsMap[componentType].(map[string]any)
	if !ok {
		return
	}

	for componentName, componentData := range typeComponents {
		key := componentName + ":" + componentType

		// If we haven't seen this component, add it.
		if _, exists := seen[key]; !exists {
			comp := map[string]any{
				fieldComponent: componentName,
				"type":         componentType,
				"stack_count":  0,
			}

			// Extract metadata from first occurrence.
			enrichUniqueComponentMetadata(comp, componentData)
			seen[key] = comp
		}

		// Increment stack count.
		if count, ok := seen[key]["stack_count"].(int); ok {
			seen[key]["stack_count"] = count + 1
		}
	}
}

// enrichUniqueComponentMetadata adds metadata fields to a unique component.
func enrichUniqueComponentMetadata(comp map[string]any, componentData any) {
	compMap, ok := componentData.(map[string]any)
	if !ok {
		setDefaultMetadataFields(comp)
		return
	}

	metadata, hasMetadata := compMap[fieldMetadata].(map[string]any)
	if hasMetadata {
		comp[fieldMetadata] = metadata
		extractMetadataFields(comp, metadata)
	} else {
		setDefaultMetadataFields(comp)
	}

	// Extract component_folder for column templates.
	if folder, ok := compMap[fieldComponentFolder].(string); ok {
		comp[fieldComponentFolder] = folder
	}
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
