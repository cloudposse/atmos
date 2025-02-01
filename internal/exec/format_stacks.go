package exec

import (
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// FormatStacksOutput formats the stacks map according to the specified formatting options
func FormatStacksOutput(stacksMap map[string]any, jsonFields string, jqQuery string, goTemplate string) (string, error) {
	// Create a default atmosConfig
	atmosConfig := schema.AtmosConfiguration{
		Logs: schema.Logs{
			Level: "Info",
		},
	}

	// If no formatting options are provided, use a pretty-printed YAML format with colors
	if jsonFields == "" && jqQuery == "" && goTemplate == "" {
		yamlBytes, err := u.ConvertToYAML(stacksMap)
		if err != nil {
			return "", fmt.Errorf("error converting to YAML: %v", err)
		}
		// Use HighlightCodeWithConfig for YAML output
		highlighted, err := u.HighlightCodeWithConfig(string(yamlBytes), atmosConfig, "yaml")
		if err != nil {
			return string(yamlBytes), nil
		}
		return highlighted, nil
	}

	// Convert to JSON if any JSON-related flags are provided
	var jsonData any = stacksMap
	if jsonFields != "" {
		// Filter JSON fields if specified
		fields := strings.Split(jsonFields, ",")
		filteredData := make(map[string]any)
		for stackName, stackInfo := range stacksMap {
			filteredStack := make(map[string]any)
			for _, field := range fields {
				if value, ok := stackInfo.(map[string]any)[field]; ok {
					filteredStack[field] = value
				}
			}
			if len(filteredStack) > 0 {
				filteredData[stackName] = filteredStack
			}
		}
		jsonData = filteredData
	}

	// Apply JQ query if specified
	if jqQuery != "" {
		result, err := u.EvaluateYqExpression(&atmosConfig, jsonData, jqQuery)
		if err != nil {
			return "", fmt.Errorf("error executing JQ query: %v", err)
		}
		jsonData = result
	}

	// Apply Go template if specified
	if goTemplate != "" {
		// TODO: Implement Go template support
		return "", fmt.Errorf("Go template support not implemented yet")
	}

	// Convert final result to JSON string
	jsonBytes, err := u.ConvertToJSON(jsonData)
	if err != nil {
		return "", fmt.Errorf("error converting to JSON: %v", err)
	}

	// Use HighlightCodeWithConfig for JSON output
	highlighted, err := u.HighlightCodeWithConfig(string(jsonBytes), atmosConfig, "json")
	if err != nil {
		return string(jsonBytes), nil
	}
	return highlighted, nil
}

// filterFields filters the input data to only include specified fields
func filterFields(data interface{}, fields string) interface{} {
	if fields == "" {
		return data
	}

	fieldList := strings.Split(fields, ",")
	if len(fieldList) == 0 {
		return data
	}

	dataMap, ok := data.(map[string]interface{})
	if !ok {
		return data
	}

	result := make(map[string]interface{})
	for _, field := range fieldList {
		field = strings.TrimSpace(field)
		if val, ok := dataMap[field]; ok {
			result[field] = val
		}
	}

	return result
} 
