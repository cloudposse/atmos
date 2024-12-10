package list

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/samber/lo"

	"github.com/cloudposse/atmos/pkg/schema"
)

// getStackComponents extracts Terraform components from the final map of stacks
func getStackComponents(stackData any, listFields []string) ([]string, error) {
	stackMap, ok := stackData.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("could not parse stacks")
	}

	componentsMap, ok := stackMap["components"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("could not parse components")
	}

	terraformComponents, ok := componentsMap["terraform"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("could not parse Terraform components")
	}

	uniqueKeys := lo.Keys(terraformComponents)
	result := make([]string, 0)

	for _, dataKey := range uniqueKeys {
		data := terraformComponents[dataKey]
		dataMap, ok := data.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("unexpected data type for component '%s'", dataKey)
		}
		rowData := make([]string, 0)
		for _, key := range listFields {
			value, found := resolveKey(dataMap, key)
			if !found {
				value = "-"
			}
			rowData = append(rowData, fmt.Sprintf("%s", value))
		}
		result = append(result, strings.Join(rowData, "\t\t"))
	}
	return result, nil
}

// resolveKey resolves a key from a map, supporting nested keys with dot notation
func resolveKey(data map[string]any, key string) (any, bool) {
	// Remove leading dot from the key (e.g., `.vars.tenant` -> `vars.tenant`)
	key = strings.TrimPrefix(key, ".")

	// Split key on `.`
	parts := strings.Split(key, ".")
	current := data

	// Traverse the map for each part
	for i, part := range parts {
		if i == len(parts)-1 {
			// Return the value for the last part
			if value, exists := current[part]; exists {
				return value, true
			}
			return nil, false
		}

		// Traverse deeper
		if nestedMap, ok := current[part].(map[string]any); ok {
			current = nestedMap
		} else {
			return nil, false
		}
	}

	return nil, false
}

// FilterAndListComponents filters and lists components based on the given stack
func FilterAndListComponents(stackFlag string, stacksMap map[string]any, listConfig schema.ListConfig) (string, error) {
	components := [][]string{}

	// Define lipgloss styles for headers and rows
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00BFFF"))
	rowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))

	header := make([]string, 0)
	listFields := make([]string, 0)

	re := regexp.MustCompile(`\{\{\s*(.*?)\s*\}\}`)

	// Extract and format headers
	for _, v := range listConfig.Columns {
		header = append(header, v.Name)
		match := re.FindStringSubmatch(v.Value)

		if len(match) > 1 {
			listFields = append(listFields, match[1])
		} else {
			return "", fmt.Errorf("invalid value format for column name %s", v.Name)
		}
	}

	// Collect components for the table
	if stackFlag != "" {
		// Filter components for the specified stack
		if stackData, ok := stacksMap[stackFlag]; ok {
			stackComponents, err := getStackComponents(stackData, listFields)
			if err != nil {
				return "", fmt.Errorf("error processing stack '%s': %w", stackFlag, err)
			}
			for _, c := range stackComponents {
				components = append(components, strings.Fields(c))
			}
		} else {
			return "", fmt.Errorf("stack '%s' not found", stackFlag)
		}
	} else {
		// Get all components from all stacks
		for _, stackData := range stacksMap {
			stackComponents, err := getStackComponents(stackData, listFields)
			if err != nil {
				continue // Skip invalid stacks
			}
			for _, c := range stackComponents {
				components = append(components, strings.Fields(c))
			}
		}
	}

	// Remove duplicates, sort, and prepare rows
	componentsMap := lo.UniqBy(components, func(item []string) string {
		return strings.Join(item, "\t")
	})
	sort.Slice(componentsMap, func(i, j int) bool {
		return strings.Join(componentsMap[i], "\t") < strings.Join(componentsMap[j], "\t")
	})

	if len(componentsMap) == 0 {
		return "No components found", nil
	}

	// Determine column widths
	colWidths := make([]int, len(header))
	for i, h := range header {
		colWidths[i] = len(h)
	}
	for _, row := range componentsMap {
		for i, field := range row {
			if len(field) > colWidths[i] {
				colWidths[i] = len(field)
			}
		}
	}

	// Format the headers
	headerRow := make([]string, len(header))
	for i, h := range header {
		headerRow[i] = headerStyle.Render(padToWidth(h, colWidths[i]))
	}
	fmt.Println(strings.Join(headerRow, "  "))

	// Format the rows
	for _, row := range componentsMap {
		formattedRow := make([]string, len(row))
		for i, field := range row {
			formattedRow[i] = rowStyle.Render(padToWidth(field, colWidths[i]))
		}
		fmt.Println(strings.Join(formattedRow, "  "))
	}

	return "", nil
}

// padToWidth ensures a string is padded to the given width
func padToWidth(str string, width int) string {
	for len(str) < width {
		str += " "
	}
	return str
}
