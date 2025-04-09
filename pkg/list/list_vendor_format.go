package list

import (
	"bytes"
	"encoding/json"
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

// formatVendorOutput formats vendor infos for output.
func formatVendorOutput(atmosConfig *schema.AtmosConfiguration, vendorInfos []VendorInfo, formatStr string) (string, error) {
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
		defaultTemplate := "{{range .vendor}}{{.Component}},{{.Type}},{{.Manifest}},{{.Folder}}\n{{end}}"
		return processTemplate(defaultTemplate, data)
	case format.FormatTable:
		return formatAsCustomTable(data, []string{ColumnNameComponent, ColumnNameType, ColumnNameManifest, ColumnNameFolder})
	default:
		return "", fmt.Errorf("unsupported format: %s", formatStr)
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
	if !strings.HasSuffix(yamlStr, "\n") {
		yamlStr += "\n"
	}

	return yamlStr, nil
}

// formatAsDelimited formats data as a delimited string (CSV, TSV).
func formatAsDelimited(data map[string]interface{}, delimiter string, columns []schema.ListColumnConfig) (string, error) {
	// Get vendor infos.
	vendorInfos, ok := data["vendor"].([]VendorInfo)
	if !ok {
		return "", fmt.Errorf("invalid vendor data")
	}

	// If no columns are configured, use default columns.
	var columnNames []string
	if len(columns) == 0 {
		columnNames = []string{ColumnNameComponent, ColumnNameType, ColumnNameManifest, ColumnNameFolder}
	} else {
		for _, col := range columns {
			columnNames = append(columnNames, col.Name)
		}
	}

	// Build header.
	var sb strings.Builder
	sb.WriteString(strings.Join(columnNames, delimiter) + "\n")

	// Build rows.
	for _, info := range vendorInfos {
		var row []string
		for _, colName := range columnNames {
			switch colName {
			case ColumnNameComponent:
				row = append(row, info.Component)
			case ColumnNameType:
				row = append(row, info.Type)
			case ColumnNameManifest:
				row = append(row, info.Manifest)
			case ColumnNameFolder:
				row = append(row, info.Folder)
			default:
				row = append(row, "")
			}
		}
		sb.WriteString(strings.Join(row, delimiter) + "\n")
	}

	return sb.String(), nil
}

// formatAsCustomTable creates a custom table format specifically for vendor listing.
func formatAsCustomTable(data map[string]interface{}, columnNames []string) (string, error) {
	// Get vendor infos.
	vendorInfos, ok := data["vendor"].([]VendorInfo)
	if !ok {
		return "", fmt.Errorf("invalid vendor data")
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
		width = 80
	}

	// Calculate column widths.
	colWidth := width / len(columnNames)
	if colWidth > 30 {
		colWidth = 30
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
