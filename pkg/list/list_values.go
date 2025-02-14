package list

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/utils"
)

const (
	DefaultCSVDelimiter = ","
	DefaultTSVDelimiter = "\t"
)

// getMapKeys returns a sorted slice of map keys
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// FilterAndListValues filters and lists component values across stacks
func FilterAndListValues(stacksMap map[string]interface{}, component, query string, includeAbstract bool, maxColumns int, format, delimiter string) (string, error) {
	if err := ValidateFormat(format); err != nil {
		return "", err
	}

	// Get terminal width for table format
	termWidth := utils.GetTerminalWidth()
	if termWidth == 0 {
		termWidth = 80 // Default width if terminal width cannot be determined
	}

	// Set default delimiters based on format
	if format == FormatCSV && delimiter == DefaultTSVDelimiter {
		delimiter = DefaultCSVDelimiter
	}

	// Filter out stacks that don't have the component
	filteredStacks := make(map[string]interface{})
	for stackName, stackData := range stacksMap {
		stack, ok := stackData.(map[string]interface{})
		if !ok {
			continue
		}

		components, ok := stack["components"].(map[string]interface{})
		if !ok {
			continue
		}

		terraform, ok := components["terraform"].(map[string]interface{})
		if !ok {
			continue
		}

		// Handle both direct and terraform/ prefixed component names
		componentName := component
		if strings.HasPrefix(component, "terraform/") {
			componentName = strings.TrimPrefix(component, "terraform/")
		}

		if componentConfig, exists := terraform[componentName]; exists {
			// Extract vars from component config
			if config, ok := componentConfig.(map[string]interface{}); ok {
				// Skip abstract components if not included
				if !includeAbstract {
					if isAbstract, ok := config["abstract"].(bool); ok && isAbstract {
						continue
					}
				}
				// Get vars from component config
				if componentVars, ok := config["vars"].(map[string]interface{}); ok {
					filteredStacks[stackName] = componentVars
				}
			}
		}
	}

	if len(filteredStacks) == 0 {
		return fmt.Sprintf("No values found for component '%s'", component), nil
	}

	// Apply JMESPath query if provided
	if query != "" {
		result := make(map[string]interface{})
		for stackName, stackData := range filteredStacks {
			// Ensure we have a valid map to query
			data, ok := stackData.(map[string]interface{})
			if !ok {
				return "", fmt.Errorf("invalid data structure for stack '%s'", stackName)
			}

			// For empty query, return all data
			if query == "" {
				result[stackName] = data
				continue
			}

			// Process the query path
			queryPath := strings.TrimPrefix(query, ".")

			// Direct access for single key
			if value, exists := data[queryPath]; exists {
				result[stackName] = value
				continue
			}

			// For nested paths, attempt to access the nested value
			parts := strings.Split(queryPath, ".")
			currentValue := interface{}(data)

			for _, part := range parts {
				if part == "" {
					continue
				}
				if mapValue, ok := currentValue.(map[string]interface{}); ok {
					if value, exists := mapValue[part]; exists {
						currentValue = value
						continue
					}
				}
				currentValue = nil
				break
			}

			// Add the value to the result if we found one
			if currentValue != nil {
				result[stackName] = currentValue
			}

		}
		filteredStacks = result
	}

	// For scalar results, create a simple key-value structure
	isScalar := true
	for _, val := range filteredStacks {
		if _, ok := val.(map[string]interface{}); ok {
			isScalar = false
			break
		}
	}

	if isScalar {
		// Create a map with stack names as keys and scalar values
		result := make(map[string]interface{})
		for stackName, val := range filteredStacks {
			result[stackName] = val
		}
		filteredStacks = result
	}

	// Get all unique keys from all stacks
	keys := make(map[string]bool)
	for _, stackData := range filteredStacks {
		if data, ok := stackData.(map[string]interface{}); ok {
			for k := range data {
				keys[k] = true
			}
		}
	}

	// Convert keys to sorted slice
	var sortedKeys []string
	for k := range keys {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	// Get sorted stack names
	var stackNames []string
	for stackName := range filteredStacks {
		stackNames = append(stackNames, stackName)
	}
	sort.Strings(stackNames)

	// Apply max columns limit
	if maxColumns > 0 && len(stackNames) > maxColumns {
		stackNames = stackNames[:maxColumns]
	}

	// Create rows with values
	var rows [][]string
	for _, key := range sortedKeys {
		row := make([]string, len(stackNames)+1)
		row[0] = key
		for i, stackName := range stackNames {
			stackData := filteredStacks[stackName]
			if data, ok := stackData.(map[string]interface{}); ok {
				if val, exists := data[key]; exists {
					// Convert value to string representation
					switch v := val.(type) {
					case string:
						row[i+1] = v
					case nil:
						row[i+1] = "null"
					default:
						jsonBytes, err := json.Marshal(v)
						if err != nil {
							row[i+1] = fmt.Sprintf("%v", v)
						} else {
							row[i+1] = string(jsonBytes)
						}
					}
				}
			}
		}
		rows = append(rows, row)
	}

	// Create header row
	header := make([]string, len(stackNames)+1)
	header[0] = "Key"
	copy(header[1:], stackNames)

	// Handle different output formats
	switch format {
	case FormatJSON, FormatYAML:
		// Create a map of stacks and their values
		result := make(map[string]interface{})
		for _, stackName := range stackNames {
			val := filteredStacks[stackName]
			// For scalar values, use them directly
			if _, ok := val.(map[string]interface{}); !ok {
				result[stackName] = val
			} else {
				// For map values, process each row
				stackValues := make(map[string]interface{})
				for _, row := range rows {
					if row[1] != "" {
						var value interface{}
						if err := json.Unmarshal([]byte(row[1]), &value); err == nil {
							stackValues[row[0]] = value
						} else {
							stackValues[row[0]] = row[1]
						}
					}
				}
				result[stackName] = stackValues
			}
		}
		if format == FormatJSON {
			jsonBytes, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return "", fmt.Errorf("error formatting JSON output: %w", err)
			}
			return string(jsonBytes), nil
		} else {
			yamlBytes, err := utils.ConvertToYAML(result)
			if err != nil {
				return "", fmt.Errorf("error formatting YAML output: %w", err)
			}
			return string(yamlBytes), nil
		}

	case FormatCSV, FormatTSV:
		var output strings.Builder
		output.WriteString(strings.Join(header, delimiter) + utils.GetLineEnding())
		for _, row := range rows {
			output.WriteString(strings.Join(row, delimiter) + utils.GetLineEnding())
		}
		return output.String(), nil

	default:
		// Calculate total table width
		totalWidth := 0
		colWidths := make([]int, len(header))

		// Calculate max width for each column
		for col := range header {
			maxWidth := len(header[col])
			for _, row := range rows {
				if len(row[col]) > maxWidth {
					maxWidth = len(row[col])
				}
			}
			colWidths[col] = maxWidth
			totalWidth += maxWidth + 3 // Add padding and border
		}

		// Check if table width exceeds terminal width
		if totalWidth > termWidth {
			return "", fmt.Errorf("the table is too wide to display properly (width: %d > %d). Try selecting a more specific range (e.g., .vars.tags instead of .vars), reducing the number of stacks, or increasing your terminal width", totalWidth, termWidth)
		}

		// If format is empty or "table", use table format
		if format == "" && exec.CheckTTYSupport() {
			// Create a styled table for TTY
			t := table.New().
				Border(lipgloss.ThickBorder()).
				BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorBorder))).
				StyleFunc(func(row, col int) lipgloss.Style {
					style := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
					if row == -1 {
						return style.Inherit(theme.Styles.CommandName).Align(lipgloss.Center)
					}
					if col == 0 {
						return style.Inherit(theme.Styles.CommandName)
					}
					return style.Inherit(theme.Styles.Description)
				}).
				Headers(header...).
				Rows(rows...)

			return t.String() + utils.GetLineEnding(), nil
		}

		// Default to simple tabular format for non-TTY or when format is explicitly "table"
		var output strings.Builder
		output.WriteString(strings.Join(header, delimiter) + utils.GetLineEnding())
		for _, row := range rows {
			output.WriteString(strings.Join(row, delimiter) + utils.GetLineEnding())
		}
		return output.String(), nil
	}
}
