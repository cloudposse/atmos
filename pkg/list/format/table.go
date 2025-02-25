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
	"github.com/pkg/errors"
)

// Constants for table formatting.
const (
	MaxColumnWidth     = 60 // Maximum width for a column.
	TableColumnPadding = 3  // Padding for table columns.
	DefaultKeyWidth    = 15 // Default base width for keys.
)

// Error variables for table formatting.
var (
	ErrTableTooWide = errors.New("the table is too wide to display properly")
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

// formatTableCellValue formats a value for display in a table cell.
// This is different from formatValue as it prioritizes compact display over completeness.
func formatTableCellValue(val interface{}) string {
	if val == nil {
		return ""
	}

	// Handle string values directly
	if str, ok := val.(string); ok {
		return truncateString(str)
	}

	// Handle different types with summary information
	v := reflect.ValueOf(val)
	switch v.Kind() {
	case reflect.Map, reflect.Array, reflect.Slice:
		return formatCollectionValue(v)
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fmt.Sprintf("%v", val)
	case reflect.Float32, reflect.Float64:
		return fmt.Sprintf("%.2f", val)
	default:
		return formatComplexValue(val)
	}
}

// truncateString truncates a string if it's longer than MaxColumnWidth.
func truncateString(str string) string {
	if len(str) > MaxColumnWidth {
		return str[:MaxColumnWidth-3] + "..."
	}
	return str
}

// formatCollectionValue formats maps, arrays and slices for display.
func formatCollectionValue(v reflect.Value) string {
	count := v.Len()
	switch v.Kind() {
	case reflect.Map:
		return fmt.Sprintf("{...} (%d keys)", count)
	case reflect.Array, reflect.Slice:
		return fmt.Sprintf("[...] (%d items)", count)
	default:
		return "{unknown collection}"
	}
}

// formatComplexValue formats complex values using JSON.
func formatComplexValue(val interface{}) string {
	jsonBytes, err := json.Marshal(val)
	if err != nil {
		return "{complex value}"
	}
	return truncateString(string(jsonBytes))
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
		return "", errors.Errorf("%s (width: %d > %d).\n\nSuggestions:\n- Use --stack to select specific stacks (examples: --stack 'plat-ue2-dev')\n- Use --query to select specific settings (example: --query '.vpc.validation')\n- Use --format json or --format yaml for complete data viewing",
			ErrTableTooWide.Error(), estimatedWidth, terminalWidth)
	}

	header := createHeader(stackKeys)
	rows := createRows(data, valueKeys, stackKeys)

	return createStyledTable(header, rows), nil
}

// calculateMaxKeyWidth determines the maximum width needed for the key column.
func calculateMaxKeyWidth(valueKeys []string) int {
	maxKeyWidth := DefaultKeyWidth // Base width assumption
	for _, key := range valueKeys {
		if len(key) > maxKeyWidth {
			maxKeyWidth = len(key)
		}
	}
	return maxKeyWidth
}

// limitWidth ensures a width doesn't exceed MaxColumnWidth.
func limitWidth(width int) int {
	if width > MaxColumnWidth {
		return MaxColumnWidth
	}
	return width
}

// getMaxValueWidth returns the maximum formatted value width in a column.
func getMaxValueWidth(stackData map[string]interface{}, valueKeys []string) int {
	maxWidth := 0

	for _, valueKey := range valueKeys {
		if val, ok := stackData[valueKey]; ok {
			formattedValue := formatTableCellValue(val)
			valueWidth := len(formattedValue)

			if valueWidth > maxWidth {
				maxWidth = valueWidth
			}
		}
	}

	return limitWidth(maxWidth)
}

// calculateStackColumnWidth calculates the width for a single stack column.
func calculateStackColumnWidth(stackName string, stackData map[string]interface{}, valueKeys []string) int {
	// Start with the width based on stack name
	columnWidth := limitWidth(len(stackName))

	// Check value widths
	valueWidth := getMaxValueWidth(stackData, valueKeys)
	if valueWidth > columnWidth {
		columnWidth = valueWidth
	}

	return columnWidth
}

// calculateEstimatedTableWidth estimates the width of the table based on content.
func calculateEstimatedTableWidth(data map[string]interface{}, valueKeys, stackKeys []string) int {
	// Calculate key column width
	maxKeyWidth := calculateMaxKeyWidth(valueKeys)
	totalWidth := maxKeyWidth + TableColumnPadding

	// Calculate width for each stack column
	for _, stackName := range stackKeys {
		var columnWidth int

		if stackData, ok := data[stackName].(map[string]interface{}); ok {
			columnWidth = calculateStackColumnWidth(stackName, stackData, valueKeys)
		} else {
			// If no stack data, just use the stack name width
			columnWidth = limitWidth(len(stackName))
		}

		totalWidth += columnWidth + TableColumnPadding
	}

	return totalWidth
}
