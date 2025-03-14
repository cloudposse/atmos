package values

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/pkg/list/errors"
	"github.com/cloudposse/atmos/pkg/utils"
)

// handleSpecialComponent processes special components like settings and metadata.
func handleSpecialComponent(stack map[string]interface{}, component string) (map[string]interface{}, bool) {
	// First check if the component exists at the top level
	if section, ok := stack[component].(map[string]interface{}); ok {
		return section, true
	}

	// If not found at the top level and component is "settings", look for it in terraform section and components.
	if component == "settings" {
		return extractAllSettings(stack)
	}

	return nil, false
}

// extractAllSettings extracts settings from all levels: component-specific, terraform-specific, and global.
func extractAllSettings(stack map[string]interface{}) (map[string]interface{}, bool) {
	allSettings := make(map[string]interface{})

	// Extract terraform-level settings
	terraformSettings, foundTerraformSettings := extractTerraformSettings(stack)
	if foundTerraformSettings {
		allSettings["terraform"] = terraformSettings
	}

	// Extract component-specific settings.
	componentSettings, foundComponentSettings := extractComponentsSettings(stack)
	if foundComponentSettings {
		allSettings["components"] = componentSettings
	}

	// Return all settings if we found any.
	foundAny := foundTerraformSettings || foundComponentSettings
	if !foundAny {
		return nil, false
	}

	return allSettings, true
}

// extractTerraformSettings extracts settings from the terraform section.
func extractTerraformSettings(stack map[string]interface{}) (interface{}, bool) {
	terraform, ok := stack["terraform"].(map[string]interface{})
	if !ok {
		return nil, false
	}

	terraformSettings, ok := terraform["settings"].(map[string]interface{})
	if !ok {
		return nil, false
	}

	return deepCopyToStringMap(terraformSettings), true
}

// extractComponentsSettings extracts settings from component-specific configurations.
func extractComponentsSettings(stack map[string]interface{}) (map[string]interface{}, bool) {
	components, ok := stack["components"].(map[string]interface{})
	if !ok {
		return nil, false
	}

	terraform, ok := components["terraform"].(map[string]interface{})
	if !ok {
		return nil, false
	}

	componentSettings := make(map[string]interface{})
	foundAny := false

	// Collect settings from all terraform components.
	for componentName, componentData := range terraform {
		if settings := extractComponentSettings(componentData); settings != nil {
			componentSettings[componentName] = settings
			foundAny = true
		}
	}

	if !foundAny {
		return nil, false
	}

	return componentSettings, true
}

// extractComponentSettings extracts settings from a component.
func extractComponentSettings(componentData interface{}) interface{} {
	comp, ok := componentData.(map[string]interface{})
	if !ok {
		return nil
	}

	settings, ok := comp["settings"].(map[string]interface{})
	if !ok {
		return nil
	}

	return deepCopyToStringMap(settings)
}

// deepCopyToStringMap creates a deep copy of a map, ensuring all keys are strings.
// This helps with JSON marshaling which requires string keys.
func deepCopyToStringMap(m interface{}) interface{} {
	switch m := m.(type) {
	case map[string]interface{}:
		copy := make(map[string]interface{})
		for k, v := range m {
			copy[k] = deepCopyToStringMap(v)
		}
		return copy
	case map[interface{}]interface{}:
		copy := make(map[string]interface{})
		for k, v := range m {
			copy[fmt.Sprintf("%v", k)] = deepCopyToStringMap(v)
		}
		return copy
	case []interface{}:
		copy := make([]interface{}, len(m))
		for i, v := range m {
			copy[i] = deepCopyToStringMap(v)
		}
		return copy
	default:
		return m
	}
}

// handleTerraformComponent processes regular terraform components.
func handleTerraformComponent(stack map[string]interface{}, component string, includeAbstract bool) (map[string]interface{}, bool) {
	components, ok := stack["components"].(map[string]interface{})
	if !ok {
		return nil, false
	}

	terraform, ok := components["terraform"].(map[string]interface{})
	if !ok {
		return nil, false
	}

	// Extract the component name from the full component path.
	componentName := component

	parts := strings.Split(component, "/")
	if len(parts) > 1 {
		componentName = parts[len(parts)-1]
	}

	comp, ok := terraform[componentName].(map[string]interface{})
	if !ok {
		return nil, false
	}

	if !includeAbstract {
		if isAbstract, ok := comp["abstract"].(bool); ok && isAbstract {
			return nil, false
		}
	}

	vars, ok := comp["vars"].(map[string]interface{})
	if !ok {
		return nil, false
	}

	return vars, true
}

// formatArrayValue converts an array to a comma-separated string.
func formatArrayValue(value interface{}) interface{} {
	if arr, ok := value.([]interface{}); ok {
		strValues := make([]string, len(arr))
		for i, v := range arr {
			strValues[i] = fmt.Sprintf("%v", v)
		}
		return strings.Join(strValues, ",")
	}
	return value
}

// ExtractStackValues implements the ValueExtractor interface for DefaultExtractor.
func (e *DefaultExtractor) ExtractStackValues(stacksMap map[string]interface{}, component string, includeAbstract bool) (map[string]interface{}, error) {
	values := make(map[string]interface{})

	for stackName, stackData := range stacksMap {
		stack, ok := stackData.(map[string]interface{})
		if !ok {
			continue
		}

		// Handle special components (settings, metadata).
		if component == "settings" || component == "metadata" {
			if section, ok := handleSpecialComponent(stack, component); ok {
				values[stackName] = section
			}
			continue
		}

		// Handle regular terraform components.
		if vars, ok := handleTerraformComponent(stack, component, includeAbstract); ok {
			values[stackName] = vars
		}
	}

	if len(values) == 0 {
		return nil, &errors.NoValuesFoundError{Component: component}
	}

	return values, nil
}

// ApplyValueQuery implements the ValueExtractor interface for DefaultExtractor.
func (e *DefaultExtractor) ApplyValueQuery(values map[string]interface{}, query string) (map[string]interface{}, error) {
	if query == "" {
		return values, nil
	}

	result := make(map[string]interface{})
	for stackName, stackData := range values {
		data, ok := stackData.(map[string]interface{})
		if !ok {
			continue
		}

		// Get value using query path.
		value := getValueFromPath(data, query)
		if value != nil {
			result[stackName] = map[string]interface{}{
				"value": formatArrayValue(value),
			}
		}
	}

	if len(result) == 0 {
		return nil, &errors.QueryError{
			Query: query,
			Cause: &errors.NoValuesFoundError{Component: "query", Query: query},
		}
	}

	return result, nil
}

// getValueFromPath gets a value from a nested map using a dot-separated path.
func getValueFromPath(data map[string]interface{}, path string) interface{} {
	if path == "" {
		return data
	}

	parts := strings.Split(strings.TrimPrefix(path, "."), ".")
	return navigatePath(data, parts)
}

// navigatePath follows a path of parts through nested data structures.
func navigatePath(data interface{}, parts []string) interface{} {
	current := data

	for _, part := range parts {
		if part == "" {
			continue
		}

		switch v := current.(type) {
		case map[string]interface{}:
			var found bool
			current, found = processMapPart(v, part)
			if !found {
				return nil
			}
		case []interface{}:
			var found bool
			current, found = processArrayPart(v, part)
			if !found {
				return current // Return array if we can't process part
			}
		default:
			return nil
		}
	}

	return current
}

// processMapPart handles traversing a map with the given part key.
func processMapPart(mapData map[string]interface{}, part string) (interface{}, bool) {
	// Check for direct key match first
	if val, exists := mapData[part]; exists {
		return val, true
	}

	// If the part contains a wildcard pattern, check all keys
	if strings.Contains(part, "*") {
		return processWildcardPattern(mapData, part)
	}

	// No match found
	return nil, false
}

// processWildcardPattern handles wildcard matching in map keys.
func processWildcardPattern(mapData map[string]interface{}, pattern string) (interface{}, bool) {
	matchFound := false
	result := make(map[string]interface{})

	for key, val := range mapData {
		matched, err := utils.MatchWildcard(pattern, key)
		if err == nil && matched {
			matchFound = true
			result[key] = val
		}
	}

	if !matchFound {
		return nil, false
	}

	// If only one match, continue with that value
	if len(result) == 1 {
		for _, val := range result {
			return val, true
		}
	}

	// Otherwise return the map of all matches
	return result, true
}

// processArrayPart handles traversing an array with the given part.
func processArrayPart(arrayData []interface{}, part string) (interface{}, bool) {
	// If part is a number, get that specific index
	if idx, err := strconv.Atoi(part); err == nil && idx >= 0 && idx < len(arrayData) {
		return arrayData[idx], true
	}

	// If array has map elements, try to access by key
	if len(arrayData) > 0 {
		if mapElement, ok := arrayData[0].(map[string]interface{}); ok {
			if val, exists := mapElement[part]; exists {
				return val, true
			}
		}
	}

	// Return false to indicate we should return the array itself
	return nil, false
}
