package format

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/cloudposse/atmos/internal/tui/templates"
	"github.com/cloudposse/atmos/pkg/terminal"
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
		// Try to expand scalar maps (like tags) into multi-line format.
		if expanded := tryExpandScalarMap(v); expanded != "" {
			return expanded
		}
		return fmt.Sprintf("{...} (%d keys)", count)
	case reflect.Array, reflect.Slice:
		// Try to expand scalar arrays into multi-line format.
		if expanded := tryExpandScalarArray(v); expanded != "" {
			return expanded
		}
		return fmt.Sprintf("[...] (%d items)", count)
	default:
		return "{unknown collection}"
	}
}

// tryExpandScalarArray attempts to expand an array of scalar values into a multi-line string.
// Returns empty string if the array contains non-scalar values or would be too wide.
func tryExpandScalarArray(v reflect.Value) string {
	if v.Len() == 0 {
		return ""
	}

	// Check if all elements are scalars (string, number, bool).
	var items []string
	maxItemWidth := 0

	for i := 0; i < v.Len(); i++ {
		elem := v.Index(i)

		// Handle interface{} wrapping.
		if elem.Kind() == reflect.Interface {
			elem = elem.Elem()
		}

		// Check if element is a scalar type.
		var itemStr string
		switch elem.Kind() {
		case reflect.String:
			itemStr = elem.String()
		case reflect.Bool:
			itemStr = fmt.Sprintf("%v", elem.Bool())
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			itemStr = fmt.Sprintf("%d", elem.Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			itemStr = fmt.Sprintf("%d", elem.Uint())
		case reflect.Float32, reflect.Float64:
			itemStr = fmt.Sprintf("%.2f", elem.Float())
		default:
			// Non-scalar element found, return empty to use placeholder format.
			return ""
		}

		items = append(items, itemStr)

		// Track the widest item to check if expansion is reasonable.
		itemWidth := lipgloss.Width(itemStr)
		if itemWidth > maxItemWidth {
			maxItemWidth = itemWidth
		}
	}

	// If the widest item exceeds a reasonable width, don't expand.
	// Use a threshold that's reasonable for table display (about 1/3 of MaxColumnWidth).
	const maxReasonableItemWidth = 20
	if maxItemWidth > maxReasonableItemWidth {
		return ""
	}

	// Join scalar items with newlines for multi-row display.
	return joinItems(items)
}

// tryExpandScalarMap attempts to expand a map with scalar values into a multi-line string.
// Returns empty string if the map contains non-scalar values or would be too wide.
func tryExpandScalarMap(v reflect.Value) string {
	if v.Len() == 0 {
		return ""
	}

	// Extract map keys and sort them for consistent ordering.
	keys := v.MapKeys()
	if len(keys) == 0 {
		return ""
	}

	// Sort keys by their string representation.
	sortedKeys := make([]string, len(keys))
	keyMap := make(map[string]reflect.Value)
	for i, key := range keys {
		keyStr := fmt.Sprintf("%v", key.Interface())
		sortedKeys[i] = keyStr
		keyMap[keyStr] = key
	}
	sort.Strings(sortedKeys)

	// Check if all values are scalars and format as "key: value".
	var items []string
	maxItemWidth := 0

	for _, keyStr := range sortedKeys {
		key := keyMap[keyStr]
		val := v.MapIndex(key)

		// Handle interface{} wrapping.
		if val.Kind() == reflect.Interface {
			val = val.Elem()
		}

		// Check if value is a scalar type.
		var valueStr string
		switch val.Kind() {
		case reflect.String:
			valueStr = val.String()
		case reflect.Bool:
			valueStr = fmt.Sprintf("%v", val.Bool())
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			valueStr = fmt.Sprintf("%d", val.Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			valueStr = fmt.Sprintf("%d", val.Uint())
		case reflect.Float32, reflect.Float64:
			valueStr = fmt.Sprintf("%.2f", val.Float())
		default:
			// Non-scalar value found, return empty to use placeholder format.
			return ""
		}

		// Format as "key: value".
		itemStr := fmt.Sprintf("%s: %s", keyStr, valueStr)
		items = append(items, itemStr)

		// Track the widest item to check if expansion is reasonable.
		itemWidth := lipgloss.Width(itemStr)
		if itemWidth > maxItemWidth {
			maxItemWidth = itemWidth
		}
	}

	// If the widest item exceeds a reasonable width, don't expand.
	// Use a threshold that's reasonable for table display (about 1/3 of MaxColumnWidth).
	const maxReasonableItemWidth = 20
	if maxItemWidth > maxReasonableItemWidth {
		return ""
	}

	// Join key-value pairs with newlines for multi-row display.
	return joinItems(items)
}

// joinItems joins array items with newlines, respecting MaxColumnWidth.
func joinItems(items []string) string {
	if len(items) == 0 {
		return ""
	}

	// Join with newlines.
	result := ""
	for i, item := range items {
		if i > 0 {
			result += "\n"
		}
		// Truncate individual items if they're too long.
		if len(item) > MaxColumnWidth {
			result += item[:MaxColumnWidth-3] + "..."
		} else {
			result += item
		}
	}

	return result
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
	contentTypeNoValue // For Go template <no value> output
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

	// Check for <no value> from Go templates.
	if value == "<no value>" {
		return contentTypeNoValue
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

	case contentTypeNoValue:
		return baseStyle.Foreground(styles.Muted.GetForeground())

	default:
		return *baseStyle
	}
}

// renderInlineMarkdown renders markdown content inline for table cells.
// Strips newlines and renders markdown formatting (bold, italic, links, code).
func renderInlineMarkdown(content string) string {
	if content == "" {
		return ""
	}

	// Create a terminal instance to detect color support.
	term := terminal.New()

	// Build glamour options for inline rendering.
	var opts []glamour.TermRendererOption

	// Use theme-aware glamour styles if color is supported.
	if term.ColorProfile() != terminal.ColorNone {
		// Get the configured theme name from atmos config if available.
		// Default to "dark" theme for better terminal compatibility.
		themeName := "dark"
		glamourStyle, err := theme.GetGlamourStyleForTheme(themeName)
		if err == nil {
			opts = append(opts, glamour.WithStylesFromJSONBytes(glamourStyle))
		} else {
			// Fallback to auto style if theme conversion fails.
			opts = append(opts, glamour.WithAutoStyle())
		}
	} else {
		// Use plain notty style for terminals without color.
		opts = append(opts, glamour.WithStylePath("notty"))
	}

	// No word wrap - we'll handle line breaks manually.
	opts = append(opts, glamour.WithWordWrap(0))

	// Create the renderer.
	renderer, err := glamour.NewTermRenderer(opts...)
	if err != nil {
		// If rendering fails, return the original content.
		return content
	}
	defer renderer.Close()

	// Render the markdown.
	rendered, err := renderer.Render(content)
	if err != nil {
		// If rendering fails, return the original content.
		return content
	}

	// Convert to single line by replacing newlines with spaces.
	// This keeps inline markdown (bold, italic, code) but removes block formatting.
	singleLine := strings.ReplaceAll(rendered, "\n", " ")

	// Collapse multiple spaces into single space.
	singleLine = regexp.MustCompile(`\s+`).ReplaceAllString(singleLine, " ")

	// Trim leading and trailing whitespace.
	return strings.TrimSpace(singleLine)
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

	// Find the index of the "Description" column if it exists.
	descriptionColIndex := -1
	for i, h := range header {
		if h == "Description" {
			descriptionColIndex = i
			break
		}
	}

	// Apply markdown rendering to Description column cells.
	processedRows := rows
	if descriptionColIndex >= 0 {
		processedRows = make([][]string, len(rows))
		for i, row := range rows {
			processedRows[i] = make([]string, len(row))
			copy(processedRows[i], row)
			if descriptionColIndex < len(row) && row[descriptionColIndex] != "" {
				// Render markdown content inline (strip block elements).
				processedRows[i][descriptionColIndex] = renderInlineMarkdown(row[descriptionColIndex])
			}
		}
	}

	// Table styling - simple and clean like version list.
	headerStyle := lipgloss.NewStyle().Bold(true)
	cellStyle := lipgloss.NewStyle()

	t := table.New().
		Headers(header...).
		Rows(processedRows...).
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
				if row >= 0 && row < len(processedRows) && col < len(processedRows[row]) {
					cellValue := processedRows[row][col]
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
		keyWidth := lipgloss.Width(key)
		if keyWidth > maxKeyWidth {
			maxKeyWidth = keyWidth
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
			// For multi-line values, get the width of the widest line.
			valueWidth := getMaxLineWidth(formattedValue)

			if valueWidth > maxWidth {
				maxWidth = valueWidth
			}
		}
	}

	return limitWidth(maxWidth)
}

// getMaxLineWidth returns the maximum visual width of any line in a multi-line string.
// Uses lipgloss.Width to properly handle ANSI codes and multi-byte characters.
func getMaxLineWidth(s string) int {
	if s == "" {
		return 0
	}

	maxWidth := 0
	lines := splitLines(s)
	for _, line := range lines {
		width := lipgloss.Width(line)
		if width > maxWidth {
			maxWidth = width
		}
	}
	return maxWidth
}

// splitLines splits a string by newlines.
func splitLines(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, "\n")
}

// calculateStackColumnWidth calculates the width for a single stack column.
func calculateStackColumnWidth(stackName string, stackData map[string]interface{}, valueKeys []string) int {
	// Start with the width based on stack name using visual width.
	columnWidth := limitWidth(lipgloss.Width(stackName))

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
