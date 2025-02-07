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
	"github.com/jmespath/go-jmespath"
)

const (
	DefaultCSVDelimiter = ","
	DefaultTSVDelimiter = "\t"
)

// FilterAndListValues filters and lists component values across stacks
func FilterAndListValues(stacksMap map[string]interface{}, component, query string, includeAbstract bool, maxColumns int, format, delimiter string) (string, error) {
	if err := ValidateFormat(format); err != nil {
		return "", err
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

		if componentConfig, exists := terraform[component]; exists {
			// Skip abstract components if not included
			if !includeAbstract {
				if config, ok := componentConfig.(map[string]interface{}); ok {
					if isAbstract, ok := config["abstract"].(bool); ok && isAbstract {
						continue
					}
				}
			}
			filteredStacks[stackName] = componentConfig
		}
	}

	if len(filteredStacks) == 0 {
		return fmt.Sprintf("No values found for component '%s'", component), nil
	}

	// Apply JMESPath query if provided
	if query != "" {
		for stackName, stackData := range filteredStacks {
			result, err := jmespath.Search(query, stackData)
			if err != nil {
				return "", fmt.Errorf("error applying query to stack '%s': %w", stackName, err)
			}
			filteredStacks[stackName] = result
		}
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
	case FormatJSON:
		// Create a map of stacks and their values
		result := make(map[string]interface{})
		for i, stackName := range stackNames {
			stackValues := make(map[string]interface{})
			for _, row := range rows {
				if row[i+1] != "" {
					var value interface{}
					if err := json.Unmarshal([]byte(row[i+1]), &value); err == nil {
						stackValues[row[0]] = value
					} else {
						stackValues[row[0]] = row[i+1]
					}
				}
			}
			result[stackName] = stackValues
		}
		jsonBytes, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return "", fmt.Errorf("error formatting JSON output: %w", err)
		}
		return string(jsonBytes), nil

	case FormatCSV, FormatTSV:
		var output strings.Builder
		output.WriteString(strings.Join(header, delimiter) + utils.GetLineEnding())
		for _, row := range rows {
			output.WriteString(strings.Join(row, delimiter) + utils.GetLineEnding())
		}
		return output.String(), nil

	default:
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
