package format

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"

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
	KeyValue           = "value"
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
		stackData := data[stackName]

		switch typedData := stackData.(type) {
		case map[string]interface{}:
			// Check if this is a vars map
			if varsData, ok := typedData["vars"].(map[string]interface{}); ok {
				for k := range varsData {
					valueKeys = append(valueKeys, k)
				}
				break
			}

			// Otherwise, use top-level keys
			for k := range typedData {
				valueKeys = append(valueKeys, k)
			}
		case []interface{}:
			return []string{KeyValue}
		default:
			return []string{KeyValue}
		}

		if len(valueKeys) > 0 {
			break
		}
	}

	sort.Strings(valueKeys)
	return valueKeys
}

// createHeader creates the table header.
func createHeader(stackKeys []string, customHeaders []string) []string {
	// If custom headers are provided, use them
	if len(customHeaders) > 0 {
		return customHeaders
	}

	// Otherwise, use the default header format
	header := []string{"Key"}
	return append(header, stackKeys...)
}

// createRows creates the table rows using value keys and stack keys.
func createRows(data map[string]interface{}, valueKeys, stackKeys []string) [][]string {
	var rows [][]string

	if len(valueKeys) == 1 && valueKeys[0] == KeyValue {
		row := []string{KeyValue}
		for _, stackName := range stackKeys {
			stackData := data[stackName]
			value := formatTableCellValue(stackData)
			row = append(row, value)
		}
		rows = append(rows, row)
		return rows
	}

	for _, valueKey := range valueKeys {
		row := []string{valueKey}
		for _, stackName := range stackKeys {
			value := ""
			if stackData, ok := data[stackName].(map[string]interface{}); ok {
				// First check if this is a vars map
				if varsData, ok := stackData["vars"].(map[string]interface{}); ok {
					if val, ok := varsData[valueKey]; ok {
						value = formatTableCellValue(val)
					}
				} else if val, ok := stackData[valueKey]; ok {
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

// cellContentType represents the type of content in a table cell.
type cellContentType int

const (
	contentTypeDefault cellContentType = iota
	contentTypeBoolean
	contentTypeNumber
	contentTypePlaceholder
)

// Regular expressions for content detection.
var (
	placeholderRegex = regexp.MustCompile(`^(\{\.\.\.}|\[\.\.\.]).*$`)
)

// detectContentType determines the content type of a cell value.
func detectContentType(value string) cellContentType {
	if value == "" {
		return contentTypeDefault
	}

	// Check for placeholders first (they contain specific patterns).
	if placeholderRegex.MatchString(value) {
		return contentTypePlaceholder
	}

	// Check for booleans.
	if value == "true" || value == "false" {
		return contentTypeBoolean
	}

	// Check for numbers (integers or floats).
	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return contentTypeNumber
	}

	return contentTypeDefault
}

// getCellStyle returns the appropriate lipgloss style for a cell based on its content.
func getCellStyle(value string, baseStyle *lipgloss.Style, styles *theme.StyleSet) lipgloss.Style {
	contentType := detectContentType(value)

	switch contentType {
	case contentTypeBoolean:
		if value == "true" {
			return baseStyle.Foreground(styles.Success.GetForeground())
		}
		return baseStyle.Foreground(styles.Error.GetForeground())

	case contentTypeNumber:
		return baseStyle.Foreground(styles.Info.GetForeground())

	case contentTypePlaceholder:
		return baseStyle.Foreground(styles.Muted.GetForeground())

	default:
		return *baseStyle
	}
}

// createStyledTable creates a styled table with headers and rows.
// Uses the same clean styling as atmos version list with width calculation and wrapping.
func CreateStyledTable(header []string, rows [][]string) string {
	// Get terminal width - use exactly what's detected.
	detectedWidth := templates.GetTerminalWidth()

	// Calculate padding more conservatively, similar to version list approach.
	// Each column needs: padding (2 chars) + separator space (1 char) = 3 chars per column
	// Plus additional margin for safety
	numColumns := len(header)
	tableBorderPadding := (numColumns * 3) + 4 // 3 per column + 4 for margins

	// Account for table borders and padding.
	tableWidth := detectedWidth - tableBorderPadding

	// Get theme-aware styles.
	styles := theme.GetCurrentStyles()

	// Table styling - simple and clean like version list.
	headerStyle := lipgloss.NewStyle().Bold(true)
	cellStyle := lipgloss.NewStyle()

	t := table.New().
		Headers(header...).
		Rows(rows...).
		BorderHeader(true).                                               // Show border under header.
		BorderTop(false).                                                 // No top border.
		BorderBottom(false).                                              // No bottom border.
		BorderLeft(false).                                                // No left border.
		BorderRight(false).                                               // No right border.
		BorderRow(false).                                                 // No row separators.
		BorderColumn(false).                                              // No column separators.
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("8"))). // Gray border.
		StyleFunc(func(row, col int) lipgloss.Style {
			switch {
			case row == table.HeaderRow:
				return headerStyle.Padding(0, 1)
			default:
				// Apply semantic styling based on cell content.
				baseStyle := cellStyle.Padding(0, 1)
				// Row indices for data start at 0, matching the rows array.
				if row >= 0 && row < len(rows) && col < len(rows[row]) {
					cellValue := rows[row][col]
					return getCellStyle(cellValue, &baseStyle, styles)
				}
				return baseStyle
			}
		})

	// Only set width and enable wrapping if we have a reasonable terminal width.
	// This prevents issues when terminal width detection fails or is unreasonably small.
	if tableWidth > 40 {
		t = t.Width(tableWidth).Wrap(true)
	}

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

	header := createHeader(stackKeys, options.CustomHeaders)
	rows := createRows(data, valueKeys, stackKeys)

	return CreateStyledTable(header, rows), nil
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
