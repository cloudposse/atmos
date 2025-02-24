package format

import (
	"sort"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/utils"
)

const (
	// TableColumnPadding is the padding between table columns
	TableColumnPadding = 3
)

// Format implements the Formatter interface for TableFormatter
func (f *TableFormatter) Format(data map[string]interface{}, options FormatOptions) (string, error) {
	// If not TTY, use CSV format
	if !options.TTY {
		csvFormatter := &DelimitedFormatter{format: FormatCSV}
		return csvFormatter.Format(data, options)
	}

	// Extract and sort keys
	var keys []string
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Apply max columns if specified
	if options.MaxColumns > 0 && len(keys) > options.MaxColumns {
		keys = keys[:options.MaxColumns]
	}

	// Get all possible value keys from the first stack
	var valueKeys []string
	for _, stackName := range keys {
		if stackData, ok := data[stackName].(map[string]interface{}); ok {
			// If it's a simple value map with "value" key
			if _, hasValue := stackData["value"]; hasValue {
				valueKeys = []string{"value"}
				break
			}
			// Otherwise, collect all keys from the map
			for k := range stackData {
				valueKeys = append(valueKeys, k)
			}
			break
		}
	}
	sort.Strings(valueKeys)

	// Create header and rows
	header := []string{"Key"}
	for _, k := range keys {
		header = append(header, k)
	}

	var rows [][]string
	for _, valueKey := range valueKeys {
		row := []string{valueKey}
		for _, stackName := range keys {
			value := ""
			if stackData, ok := data[stackName].(map[string]interface{}); ok {
				if val, ok := stackData[valueKey]; ok {
					value = formatValue(val)
				}
			}
			row = append(row, value)
		}
		rows = append(rows, row)
	}

	// This is the styled table using lipgloss
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

	return t.String() + utils.GetLineEnding(), nil
}
