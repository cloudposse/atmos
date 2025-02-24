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

	// If not found at the top level, look for settings within components
	if component == "settings" {
		allSettings := make(map[string]interface{})

		// Try to navigate to the terraform components we cannot do too much here to avoid nesting for now.
		if components, ok := stack["components"].(map[string]interface{}); ok {
			if terraform, ok := components["terraform"].(map[string]interface{}); ok {
				// Collect settings from all terraform components
				for componentName, componentData := range terraform {
					if comp, ok := componentData.(map[string]interface{}); ok {
						if settings, ok := comp["settings"].(map[string]interface{}); ok {
							allSettings[componentName] = deepCopyToStringMap(settings)
						}
					}
				}

				// Return all settings if we found any
				if len(allSettings) > 0 {
					return allSettings, true
				}
			}
		}
	}

	return nil, false
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

	componentName := strings.TrimPrefix(component, "terraform/")
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
	current := interface{}(data)

	for _, part := range parts {
		if part == "" {
			continue
		}

		switch v := current.(type) {
		case map[string]interface{}:
			// Check for direct key match first
			if val, exists := v[part]; exists {
				current = val
				continue
			}

			// If the part contains a wildcard pattern, check all keys
			if strings.Contains(part, "*") {
				matchFound := false
				result := make(map[string]interface{})

				for key, val := range v {
					matched, err := utils.MatchWildcard(part, key)
					if err == nil && matched {
						matchFound = true
						result[key] = val
					}
				}

				if matchFound {
					// If only one match, continue with that value
					if len(result) == 1 {
						for _, val := range result {
							current = val
						}
					} else {
						// Otherwise return the map of all matches
						current = result
					}
					continue
				}
			}

			// No match found
			return nil
		case []interface{}:
			// If part is a number, get that specific index.
			if idx, err := strconv.Atoi(part); err == nil && idx >= 0 && idx < len(v) {
				current = v[idx]
			} else {
				// If part is not a number and we have an array, return the entire array.
				if val, exists := v[0].(map[string]interface{})[part]; exists {
					current = val
				} else {
					return current // Return the entire array if we're trying to access it directly.
				}
			}
		default:
			return nil
		}
	}

	return current
}
