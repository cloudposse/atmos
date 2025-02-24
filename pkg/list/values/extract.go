package values

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/pkg/list/errors"
)

// ExtractStackValues implements the ValueExtractor interface for DefaultExtractor
func (e *DefaultExtractor) ExtractStackValues(stacksMap map[string]interface{}, component string, includeAbstract bool) (map[string]interface{}, error) {
	values := make(map[string]interface{})

	for stackName, stackData := range stacksMap {
		stack, ok := stackData.(map[string]interface{})
		if !ok {
			continue
		}

		// Handle special components (settings, metadata)
		if component == "settings" || component == "metadata" {
			if section, ok := stack[component].(map[string]interface{}); ok {
				values[stackName] = section
			}
			continue
		}

		// Handle regular components
		if components, ok := stack["components"].(map[string]interface{}); ok {
			if terraform, ok := components["terraform"].(map[string]interface{}); ok {
				componentName := strings.TrimPrefix(component, "terraform/")
				if comp, ok := terraform[componentName].(map[string]interface{}); ok {
					// Skip abstract components if not included
					if !includeAbstract {
						if isAbstract, ok := comp["abstract"].(bool); ok && isAbstract {
							continue
						}
					}
					if vars, ok := comp["vars"].(map[string]interface{}); ok {
						values[stackName] = vars
					}
				}
			}
		}
	}

	if len(values) == 0 {
		return nil, &errors.NoValuesFoundError{Component: component}
	}

	return values, nil
}

// ApplyValueQuery implements the ValueExtractor interface for DefaultExtractor
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

		// Get value using query path
		value := getValueFromPath(data, query)
		if value != nil {
			// Format array values as strings
			if arr, ok := value.([]interface{}); ok {
				strValues := make([]string, len(arr))
				for i, v := range arr {
					strValues[i] = fmt.Sprintf("%v", v)
				}
				value = strings.Join(strValues, ",")
			}
			result[stackName] = map[string]interface{}{
				"value": value,
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

// getValueFromPath gets a value from a nested map using a dot-separated path
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
			if val, exists := v[part]; exists {
				current = val
			} else {
				return nil
			}
		case []interface{}:
			// If part is a number, get that specific index
			if idx, err := strconv.Atoi(part); err == nil && idx >= 0 && idx < len(v) {
				current = v[idx]
			} else {
				// If part is not a number and we have an array, return the entire array
				if val, exists := v[0].(map[string]interface{})[part]; exists {
					current = val
				} else {
					return current // Return the entire array if we're trying to access it directly
				}
			}
		default:
			return nil
		}
	}

	return current
}
