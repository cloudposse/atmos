package list

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"text/template"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/utils"
)

// BuildStackMetadata creates a standardized stack metadata map from raw stack data.
// It extracts and organizes variables, components, and other stack properties into a consistent structure.
func BuildStackMetadata(stackName string, stackData map[string]any) map[string]any {
	stackInfo := map[string]any{
		"atmos_stack":      stackName,
		"atmos_stack_file": stackData["atmos_stack_file"],
		"vars":             make(map[string]any),
		"components":       make(map[string]any),
	}

	// Extract variables from stack level
	if stackVars, ok := stackData["vars"].(map[string]any); ok {
		for k, v := range stackVars {
			if v != nil {
				stackInfo["vars"].(map[string]any)[k] = v
			}
		}
	}

	// Copy components with their full structure
	if components, ok := stackData["components"].(map[string]any); ok {
		for compType, compSection := range components {
			if sectionMap, ok := compSection.(map[string]any); ok {
				if _, exists := stackInfo["components"].(map[string]any)[compType]; !exists {
					stackInfo["components"].(map[string]any)[compType] = make(map[string]any)
				}

				// Copy all component configurations
				for compName, compConfig := range sectionMap {
					if configMap, ok := compConfig.(map[string]any); ok {
						stackInfo["components"].(map[string]any)[compType].(map[string]any)[compName] = configMap
					}
				}
			}
		}
	}

	// Extract stage from stack name if not set in vars
	if _, ok := stackInfo["vars"].(map[string]any)["stage"]; !ok {
		// Only set stage from stack name if it's not already set in vars
		if stackName != stackInfo["atmos_stack_file"].(string) {
			stackInfo["vars"].(map[string]any)["stage"] = stackName
		}
	}

	// Copy other stack configuration
	for k, v := range stackData {
		if k != "vars" && k != "components" {
			stackInfo[k] = v
		}
	}

	return stackInfo
}

// ContainsComponent checks if a stack's components map contains the specified component.
func ContainsComponent(components map[string]any, targetComponent string) bool {
	for _, section := range components {
		if compSection, ok := section.(map[string]any); ok {
			if _, exists := compSection[targetComponent]; exists {
				return true
			}
		}
	}
	return false
}

// HasNonEmptyTemplateValues checks if a column template produces any non-empty values
// when executed against a slice of stack data.
func HasNonEmptyTemplateValues(columnName, templateStr string, stacks []map[string]any) bool {
	// Stack is always shown
	if columnName == "Stack" {
		return true
	}

	tmpl, err := template.New(columnName).Parse(templateStr)
	if err != nil {
		return false
	}

	// Check if any stack has a non-empty value for this column
	for _, stack := range stacks {
		var buf strings.Builder
		if err := tmpl.Execute(&buf, stack); err != nil {
			continue
		}
		value := buf.String()
		if value != "" && value != "<nil>" && value != "<no value>" {
			return true
		}
	}
	return false
}

// FilterAndListStacks filters and lists stacks based on the given configuration
func FilterAndListStacks(stacksMap map[string]any, component string, listConfig schema.ListConfig, format string, delimiter string) (string, error) {
	if err := ValidateFormat(format); err != nil {
		return "", err
	}

	if format == "" && listConfig.Format != "" {
		if err := ValidateFormat(listConfig.Format); err != nil {
			return "", err
		}
		format = listConfig.Format
	}

	var filteredStacks []map[string]any

	// Filter and process stacks
	for stackName, stackData := range stacksMap {
		v2, ok := stackData.(map[string]any)
		if !ok {
			continue
		}

		if component != "" {
			// Only include stacks with the specified component
			if components, ok := v2["components"].(map[string]any); ok && ContainsComponent(components, component) {
				stackInfo := BuildStackMetadata(stackName, v2)
				filteredStacks = append(filteredStacks, stackInfo)
			}
		} else {
			// Include all stacks when no component filter is specified
			stackInfo := BuildStackMetadata(stackName, v2)
			filteredStacks = append(filteredStacks, stackInfo)
		}
	}

	if len(filteredStacks) == 0 {
		if component != "" {
			return fmt.Sprintf("No stacks found for component '%s'"+"\n", component), nil
		}
		return "No stacks found\n", nil
	}

	// Sort stacks by name
	sort.Slice(filteredStacks, func(i, j int) bool {
		return filteredStacks[i]["atmos_stack"].(string) < filteredStacks[j]["atmos_stack"].(string)
	})

	// If no columns are configured, use default columns
	if len(listConfig.Columns) == 0 {
		// Define all possible columns
		allColumns := []schema.ListColumnConfig{
			{Name: "Stack", Value: "{{ .atmos_stack }}"},
			{Name: "Tenant", Value: "{{ index .vars \"tenant\" }}"},
			{Name: "Environment", Value: "{{ index .vars \"environment\" }}"},
			{Name: "Stage", Value: "{{ index .vars \"stage\" }}"},
			{Name: "File", Value: "{{ .atmos_stack_file }}"},
		}

		// Filter out columns with no values
		var activeColumns []schema.ListColumnConfig
		for _, col := range allColumns {
			if HasNonEmptyTemplateValues(col.Name, col.Value, filteredStacks) {
				activeColumns = append(activeColumns, col)
			}
		}

		listConfig.Columns = activeColumns
	}

	// Prepare headers and rows
	headers := make([]string, len(listConfig.Columns))
	rows := make([][]string, len(filteredStacks))

	for i, col := range listConfig.Columns {
		headers[i] = col.Name
	}

	// Pre-parse templates for better performance
	type columnTemplate struct {
		name     string
		template *template.Template
	}

	templates := make([]columnTemplate, len(listConfig.Columns))
	for i, col := range listConfig.Columns {
		tmpl, err := template.New(col.Name).Parse(col.Value)
		if err != nil {
			return "", fmt.Errorf("error parsing template for column %s: %w", col.Name, err)
		}
		templates[i] = columnTemplate{name: col.Name, template: tmpl}
	}

	// Process each stack and populate rows
	for i, stack := range filteredStacks {
		row := make([]string, len(listConfig.Columns))
		for j, tmpl := range templates {
			var buf strings.Builder
			if err := tmpl.template.Execute(&buf, stack); err != nil {
				return "", fmt.Errorf("error executing template for column %s: %w", tmpl.name, err)
			}
			// Just use the raw string value
			row[j] = buf.String()
		}
		rows[i] = row
	}

	// Handle different output formats
	switch format {
	case FormatJSON:
		// Convert to JSON format using only non-empty columns
		var stacks []map[string]string
		for _, row := range rows {
			s := make(map[string]string)
			for i, header := range headers {
				// Only include non-empty values in JSON output
				if row[i] != "" {
					// Use raw value without any color formatting for JSON output
					s[strings.ToLower(header)] = row[i]
				}
			}
			stacks = append(stacks, s)
		}
		// Use plain JSON marshaling without any color formatting
		jsonBytes, err := json.MarshalIndent(stacks, "", "  ")
		if err != nil {
			return "", fmt.Errorf("error formatting JSON output: %w", err)
		}
		return string(jsonBytes) + utils.GetLineEnding(), nil

	case FormatCSV, FormatTSV:
		var nonEmptyHeaders []string
		var nonEmptyColumnIndexes []int

		for i, header := range headers {
			hasValue := false
			for _, row := range rows {
				if row[i] != "" {
					hasValue = true
					break
				}
			}
			if hasValue {
				nonEmptyHeaders = append(nonEmptyHeaders, header)
				nonEmptyColumnIndexes = append(nonEmptyColumnIndexes, i)
			}
		}

		csvDelimiter := delimiter
		if delimiter == "\t" || delimiter == "" {
			switch format {
			case FormatCSV:
				csvDelimiter = ","
			case FormatTSV:
				csvDelimiter = "\t"
			}
		}

		var output strings.Builder

		switch format {
		case FormatCSV:
			// Use encoding/csv for proper CSV handling
			writer := csv.NewWriter(&output)
			writer.Comma = rune(csvDelimiter[0])

			if err := writer.Write(nonEmptyHeaders); err != nil {
				return "", fmt.Errorf("error writing CSV headers: %w", err)
			}

			for _, row := range rows {
				csvRow := make([]string, len(nonEmptyColumnIndexes))
				for j, i := range nonEmptyColumnIndexes {
					csvRow[j] = row[i]
				}
				if err := writer.Write(csvRow); err != nil {
					return "", fmt.Errorf("error writing CSV row: %w", err)
				}
			}
			writer.Flush()
			if err := writer.Error(); err != nil {
				return "", fmt.Errorf("error flushing CSV writer: %w", err)
			}

		case FormatTSV:
			// For TSV, replace tabs with spaces in values and use tab as delimiter
			// Write headers
			for i, header := range nonEmptyHeaders {
				if i > 0 {
					output.WriteString("\t")
				}
				output.WriteString(strings.ReplaceAll(header, "\t", " "))
			}
			output.WriteString(utils.GetLineEnding())

			// Write rows
			for _, row := range rows {
				for j, i := range nonEmptyColumnIndexes {
					if j > 0 {
						output.WriteString("\t")
					}
					output.WriteString(strings.ReplaceAll(row[i], "\t", " "))
				}
				output.WriteString(utils.GetLineEnding())
			}
		}
		return output.String(), nil

	default:
		// For non-TTY output with no specific format, default to CSV
		if !term.IsTTYSupportForStdout() || format == "csv" {
			var output strings.Builder
			writer := csv.NewWriter(&output)
			writer.Comma = ','

			if err := writer.Write(headers); err != nil {
				return "", fmt.Errorf("error writing CSV headers: %w", err)
			}

			for _, row := range rows {
				if err := writer.Write(row); err != nil {
					return "", fmt.Errorf("error writing CSV row: %w", err)
				}
			}
			writer.Flush()
			if err := writer.Error(); err != nil {
				return "", fmt.Errorf("error flushing CSV writer: %w", err)
			}
			return output.String(), nil
		}

		// For TTY output or when format is not specified, use table format
		t := table.New()

		if term.IsTTYSupportForStdout() {
			t = t.Border(lipgloss.ThickBorder()).
				BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorBorder))).
				StyleFunc(func(row, col int) lipgloss.Style {
					style := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
					if row == -1 {
						return style.Inherit(theme.Styles.CommandName).Align(lipgloss.Center)
					}
					return style.Inherit(theme.Styles.Description)
				})
		} else {
			t = t.Border(lipgloss.HiddenBorder()).
				StyleFunc(func(row, col int) lipgloss.Style {
					return lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
				})
		}

		t = t.Headers(headers...).Rows(rows...)

		return t.String() + utils.GetLineEnding(), nil
	}
}
