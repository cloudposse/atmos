package format

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/cloudposse/atmos/internal/tui/templates"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/utils"
)

const (
	// TableColumnPadding is the padding between table columns
	TableColumnPadding = 3
	// MaxColumnWidth is the maximum width for any column in the table
	MaxColumnWidth = 30
)

// extractAndSortKeys extracts and sorts the keys from the data map.
func extractAndSortKeys(data map[string]interface{}, maxColumns int) []string {
	var keys []string
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	if maxColumns > 0 && len(keys) > maxColumns {
		keys = keys[:maxColumns]
	}

	return keys
}

// extractValueKeys extracts value keys from the first stack data.
func extractValueKeys(data map[string]interface{}, stackKeys []string) []string {
	var valueKeys []string
	for _, stackName := range stackKeys {
		if stackData, ok := data[stackName].(map[string]interface{}); ok {
			// If it's a simple value map with "value" key, return it.
			if _, hasValue := stackData["value"]; hasValue {
				return []string{"value"}
			}
			// Otherwise, collect all keys from the map
			for k := range stackData {
				valueKeys = append(valueKeys, k)
			}
			break
		}
	}
	sort.Strings(valueKeys)
	return valueKeys
}

// createHeader creates the table header.
func createHeader(stackKeys []string) []string {
	header := []string{"Key"}
	return append(header, stackKeys...)
}

// createRows creates the table rows using value keys and stack keys.
func createRows(data map[string]interface{}, valueKeys, stackKeys []string) [][]string {
	var rows [][]string
	for _, valueKey := range valueKeys {
		row := []string{valueKey}
		for _, stackName := range stackKeys {
			value := ""
			if stackData, ok := data[stackName].(map[string]interface{}); ok {
				if val, ok := stackData[valueKey]; ok {
					value = formatTableCellValue(val)
				}
			}
			row = append(row, value)
		}
		rows = append(rows, row)
	}
	return rows
}

// formatTableCellValue formats a value specifically for table cell display
// This is different from formatValue as it prioritizes compact display over completeness
func formatTableCellValue(val interface{}) string {
	if val == nil {
		return ""
	}

	// Handle string values directly
	if str, ok := val.(string); ok {
		if len(str) > MaxColumnWidth {
			return str[:MaxColumnWidth-3] + "..."
		}
		return str
	}

	// Handle different types with summary information
	v := reflect.ValueOf(val)
	switch v.Kind() {
	case reflect.Map:
		count := v.Len()
		return fmt.Sprintf("{...} (%d keys)", count)
	case reflect.Array, reflect.Slice:
		count := v.Len()
		return fmt.Sprintf("[...] (%d items)", count)
	case reflect.Bool:
		return fmt.Sprintf("%v", val)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fmt.Sprintf("%v", val)
	case reflect.Float32, reflect.Float64:
		return fmt.Sprintf("%.2f", val)
	default:
		jsonBytes, err := json.Marshal(val)
		if err != nil {
			return "{complex value}"
		}
		summary := string(jsonBytes)
		if len(summary) > MaxColumnWidth {
			return summary[:MaxColumnWidth-3] + "..."
		}
		return summary
	}
}

// createStyledTable creates a styled table with headers and rows.
func createStyledTable(header []string, rows [][]string) string {
	t := table.New().
		Border(lipgloss.ThickBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorBorder))).
		StyleFunc(func(row, col int) lipgloss.Style {
			style := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
			if row == -1 {
				return style.
					Foreground(lipgloss.Color(theme.ColorGreen)).
					Bold(true).
					Align(lipgloss.Center)
			}
			return style.Inherit(theme.Styles.Description)
		}).
		Headers(header...).
		Rows(rows...)

	return t.String() + utils.GetLineEnding()
}

// Format implements the Formatter interface for TableFormatter.
func (f *TableFormatter) Format(data map[string]interface{}, options FormatOptions) (string, error) {
	if !options.TTY {
		// to ensure consistency
		csvFormatter := &DelimitedFormatter{format: FormatCSV}
		return csvFormatter.Format(data, options)
	}

	// Get stack keys
	stackKeys := extractAndSortKeys(data, options.MaxColumns)
	valueKeys := extractValueKeys(data, stackKeys)

	// Estimate table width before creating it
	estimatedWidth := calculateEstimatedTableWidth(data, valueKeys, stackKeys)
	terminalWidth := templates.GetTerminalWidth()

	// Check if the table would be too wide
	if estimatedWidth > terminalWidth {
		return "", fmt.Errorf("the table is too wide to display properly (width: %d > %d).\n\nSuggestions:\n- Use --stack to select specific stacks (examples: --stack 'plat-ue2-dev')\n- Use --query to select specific settings (example: --query '.vpc.validation')\n- Use --format json or --format yaml for complete data viewing",
			estimatedWidth, terminalWidth)
	}

	header := createHeader(stackKeys)
	rows := createRows(data, valueKeys, stackKeys)

	return createStyledTable(header, rows), nil
}

// calculateEstimatedTableWidth estimates the width of the table based on content
func calculateEstimatedTableWidth(data map[string]interface{}, valueKeys, stackKeys []string) int {
	// Start with the width for the "Key" column, assuming 15 chars as a base width
	maxKeyWidth := 15
	for _, key := range valueKeys {
		if len(key) > maxKeyWidth {
			maxKeyWidth = len(key)
		}
	}

	// Add column padding for the key column
	totalWidth := maxKeyWidth + TableColumnPadding

	// For each stack column, estimate width and add padding
	for _, stackName := range stackKeys {
		columnWidth := len(stackName)
		if columnWidth > MaxColumnWidth {
			columnWidth = MaxColumnWidth
		}

		// Check actual content width for this stack (with capped width)
		if stackData, ok := data[stackName].(map[string]interface{}); ok {
			for _, valueKey := range valueKeys {
				if val, ok := stackData[valueKey]; ok {
					formattedValue := formatTableCellValue(val)
					valueLength := len(formattedValue)
					if valueLength > columnWidth && valueLength <= MaxColumnWidth {
						columnWidth = valueLength
					} else if valueLength > MaxColumnWidth {
						columnWidth = MaxColumnWidth
					}
				}
			}
		}

		totalWidth += columnWidth + TableColumnPadding
	}

	return totalWidth
}
