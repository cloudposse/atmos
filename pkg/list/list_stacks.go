package list

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/internal/tui/templates"
	"github.com/samber/lo"
)

// FilterAndListStacks filters stacks by the given component and returns the output based on the format and template
func FilterAndListStacks(stacksMap map[string]any, component string, jsonFields string, jqQuery string, goTemplate string) (string, error) {
	var result any

	if component != "" {
		// Filter stacks by component
		filteredStacks := []string{}
		for stackName, stackData := range stacksMap {
			v2, ok := stackData.(map[string]any)
			if !ok {
				continue
			}
			components, ok := v2["components"].(map[string]any)
			if !ok {
				continue
			}
			terraform, ok := components["terraform"].(map[string]any)
			if !ok {
				continue
			}
			if _, exists := terraform[component]; exists {
				filteredStacks = append(filteredStacks, stackName)
			}
		}

		if len(filteredStacks) == 0 {
			return fmt.Sprintf("No stacks found for component '%s'"+"\n", component), nil
		}
		sort.Strings(filteredStacks)
		
		// If JSON output is requested, convert to map
		if jsonFields != "" || jqQuery != "" || goTemplate != "" {
			stacksOutput := make(map[string]interface{})
			for _, stack := range filteredStacks {
				stacksOutput[stack] = filterFields(stacksMap[stack], jsonFields)
			}
			result = stacksOutput
		} else {
			return strings.Join(filteredStacks, "\n") + "\n", nil
		}
	} else {
		// List all stacks
		if jsonFields != "" || jqQuery != "" || goTemplate != "" {
			if jsonFields != "" {
				filteredMap := make(map[string]interface{})
				for stack, data := range stacksMap {
					filteredMap[stack] = filterFields(data, jsonFields)
				}
				result = filteredMap
			} else {
				result = stacksMap
			}
		} else {
			stacks := lo.Keys(stacksMap)
			sort.Strings(stacks)
			return strings.Join(stacks, "\n") + "\n", nil
		}
	}

	// Process template if provided
	if jqQuery != "" {
		return templates.ProcessJSONWithTemplate(result, jqQuery)
	}
	if goTemplate != "" {
		return templates.ProcessWithGoTemplate(result, goTemplate)
	}

	// If JSON fields are specified but no template, return JSON string
	if jsonFields != "" {
		return templates.ProcessJSONWithTemplate(result, ".")
	}

	return "", fmt.Errorf("unexpected condition in FilterAndListStacks")
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
