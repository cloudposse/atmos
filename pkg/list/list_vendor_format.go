package list

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"text/template"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/cloudposse/atmos/internal/tui/templates"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"gopkg.in/yaml.v3"
)

const (
	// NewLine is the newline character.
	NewLine = "\n"
	// DefaultTerminalWidth is the default terminal width.
	DefaultTerminalWidth = 80
	// MaxColumnWidth is the maximum width for a column.
	MaxColumnWidth = 30
)

var (
	// ErrUnsupportedFormat is returned when an unsupported format is specified.
	ErrUnsupportedFormat = errors.New("unsupported format")
	// ErrInvalidVendorData is returned when vendor data is invalid.
	ErrInvalidVendorData = errors.New("invalid vendor data")
)

// formatVendorOutput formats vendor infos for output.
func formatVendorOutput(vendorInfos []VendorInfo, formatStr string) (string, error) {
	if len(vendorInfos) == 0 {
		return "No vendor configurations found", nil
	}

	// Convert to map for template processing.
	data := map[string]interface{}{
		"vendor": vendorInfos,
	}

	// Process based on format.
	switch format.Format(formatStr) {
	case format.FormatJSON:
		return formatAsJSON(data)
	case format.FormatYAML:
		return formatAsYAML(data)
	case format.FormatCSV:
		return formatAsDelimited(data, ",", []schema.ListColumnConfig{})
	case format.FormatTSV:
		return formatAsDelimited(data, "\t", []schema.ListColumnConfig{})
	case format.FormatTemplate:
		// Use a default template for vendor output
		defaultTemplate := "{{range .vendor}}{{.Component}},{{.Type}},{{.Manifest}},{{.Folder}}" + NewLine + "{{end}}"
		return processTemplate(defaultTemplate, data)
	case format.FormatTable:
		return formatAsCustomTable(data, []string{ColumnNameComponent, ColumnNameType, ColumnNameManifest, ColumnNameFolder})
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedFormat, formatStr)
	}
}

// processTemplate processes a template string with the given data.
func processTemplate(templateStr string, data map[string]interface{}) (string, error) {
	tmpl, err := template.New("output").Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("error parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("error executing template: %w", err)
	}

	return buf.String(), nil
}

// formatAsJSON formats data as JSON.
func formatAsJSON(data map[string]interface{}) (string, error) {
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", fmt.Errorf("error marshaling to JSON: %w", err)
	}

	// Add newline at end.
	jsonStr := string(jsonBytes)
	if !strings.HasSuffix(jsonStr, "\n") {
		jsonStr += "\n"
	}

	return jsonStr, nil
}

// formatAsYAML formats data as YAML.
func formatAsYAML(data map[string]interface{}) (string, error) {
	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("error marshaling to YAML: %w", err)
	}

	// Add newline at end.
	yamlStr := string(yamlBytes)
	if !strings.HasSuffix(yamlStr, NewLine) {
		yamlStr += NewLine
	}

	return yamlStr, nil
}

// getColumnNames returns the column names for the delimited output.
func getColumnNames(columns []schema.ListColumnConfig) []string {
	if len(columns) == 0 {
		return []string{ColumnNameComponent, ColumnNameType, ColumnNameManifest, ColumnNameFolder}
	}

	var columnNames []string
	for _, col := range columns {
		columnNames = append(columnNames, col.Name)
	}
	return columnNames
}

// getRowValue returns the value for a specific column in a vendor info.
func getRowValue(info VendorInfo, colName string) string {
	switch colName {
	case ColumnNameComponent:
		return info.Component
	case ColumnNameType:
		return info.Type
	case ColumnNameManifest:
		return info.Manifest
	case ColumnNameFolder:
		return info.Folder
	default:
		return ""
	}
}

// formatAsDelimited formats data as a delimited string (CSV, TSV).
func formatAsDelimited(data map[string]interface{}, delimiter string, columns []schema.ListColumnConfig) (string, error) {
	// Get vendor infos.
	vendorInfos, ok := data["vendor"].([]VendorInfo)
	if !ok {
		return "", ErrInvalidVendorData
	}

	// Get column names.
	columnNames := getColumnNames(columns)

	// Build header.
	var sb strings.Builder
	sb.WriteString(strings.Join(columnNames, delimiter) + NewLine)

	// Build rows.
	for _, info := range vendorInfos {
		var row []string
		for _, colName := range columnNames {
			row = append(row, getRowValue(info, colName))
		}
		sb.WriteString(strings.Join(row, delimiter) + NewLine)
	}

	return sb.String(), nil
}

// formatAsCustomTable creates a custom table format specifically for vendor listing.
func formatAsCustomTable(data map[string]interface{}, columnNames []string) (string, error) {
	// Get vendor infos.
	vendorInfos, ok := data["vendor"].([]VendorInfo)
	if !ok {
		return "", ErrInvalidVendorData
	}

	// Create rows.
	var rows [][]string
	for _, info := range vendorInfos {
		row := []string{
			info.Component,
			info.Type,
			info.Manifest,
			info.Folder,
		}
		rows = append(rows, row)
	}

	// Create table.
	width := templates.GetTerminalWidth()
	if width <= 0 {
		width = DefaultTerminalWidth
	}

	// Create table with lipgloss.
	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorBorder))).
		Headers(columnNames...).
		Width(width).
		Rows(rows...)

	// Render the table
	return t.Render(), nil
}
