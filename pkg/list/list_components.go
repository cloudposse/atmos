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

const (
	HeaderColor = "#00BFFF"
	RowColor    = "#FFFFFF"
)

type tableData struct {
	header    []string
	rows      [][]string
	colWidths []int
}

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

// parseColumns extracts the header and list fields from the listConfig
func parseColumns(listConfig schema.ListConfig) ([]string, []string, error) {
	header := make([]string, 0)
	listFields := make([]string, 0)
	re := regexp.MustCompile(`\{\{\s*(.*?)\s*\}\}`)

	for _, col := range listConfig.Columns {
		header = append(header, col.Name)
		match := re.FindStringSubmatch(col.Value)
		if len(match) > 1 {
			listFields = append(listFields, match[1])
		} else {
			return nil, nil, fmt.Errorf("invalid value format for column name %s", col.Name)
		}
	}
	return header, listFields, nil
}

// collectComponents gathers components for the specified stack or all stacks
func collectComponents(stackFlag string, stacksMap map[string]any, listFields []string) ([][]string, error) {
	components := [][]string{}

	if stackFlag != "" {
		// Filter components for the specified stack
		if stackData, ok := stacksMap[stackFlag]; ok {
			stackComponents, err := getStackComponents(stackData, listFields)
			if err != nil {
				return nil, fmt.Errorf("error processing stack '%s': %w", stackFlag, err)
			}
			for _, c := range stackComponents {
				components = append(components, strings.Fields(c))
			}
		} else {
			return nil, fmt.Errorf("stack '%s' not found", stackFlag)
		}
	} else {
		// Collect components from all stacks
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
	return components, nil
}

// processComponents deduplicates, sorts, and calculates column widths
func processComponents(header []string, components [][]string) ([][]string, []int) {
	uniqueComponents := lo.UniqBy(components, func(item []string) string {
		return strings.Join(item, "\t")
	})
	sort.Slice(uniqueComponents, func(i, j int) bool {
		return strings.Join(uniqueComponents[i], "\t") < strings.Join(uniqueComponents[j], "\t")
	})

	colWidths := make([]int, len(header))
	for i, h := range header {
		colWidths[i] = len(h)
	}
	for _, row := range uniqueComponents {
		for i, field := range row {
			if len(field) > colWidths[i] {
				colWidths[i] = len(field)
			}
		}
	}

	return uniqueComponents, colWidths
}

// formatTable generates the formatted table
func formatTable(data tableData) {
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(HeaderColor))
	rowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(RowColor))

	// Format and print headers
	headerRow := make([]string, len(data.header))
	for i, h := range data.header {
		headerRow[i] = headerStyle.Render(padToWidth(h, data.colWidths[i]))
	}
	fmt.Println(strings.Join(headerRow, "  "))

	// Format and print rows
	for _, row := range data.rows {
		formattedRow := make([]string, len(row))
		for i, field := range row {
			formattedRow[i] = rowStyle.Render(padToWidth(field, data.colWidths[i]))
		}
		fmt.Println(strings.Join(formattedRow, "  "))
	}
}

// padToWidth ensures a string is padded to the given width
func padToWidth(str string, width int) string {
	for len(str) < width {
		str += " "
	}
	return str
}

// FilterAndListComponents orchestrates the process
func FilterAndListComponents(stackFlag string, stacksMap map[string]any, listConfig schema.ListConfig) (string, error) {
	// Step 1: Parse columns
	header, listFields, err := parseColumns(listConfig)
	if err != nil {
		return "", err
	}

	// Step 2: Collect components
	components, err := collectComponents(stackFlag, stacksMap, listFields)
	if err != nil {
		return "", err
	}

	// Step 3: Process components
	processedComponents, colWidths := processComponents(header, components)
	if len(processedComponents) == 0 {
		return "No components found", nil
	}

	// Step 4: Format and display table
	data := tableData{
		header:    header,
		rows:      processedComponents,
		colWidths: colWidths,
	}
	formatTable(data)

	return "", nil
}
