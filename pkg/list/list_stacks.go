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
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/utils"
)

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

	// Helper function to create stack info
	createStackInfo := func(stackName string, v2 map[string]any) map[string]any {
		stackInfo := map[string]any{
			"atmos_stack":      stackName,
			"atmos_stack_file": v2["atmos_stack_file"],
			"vars":             make(map[string]any),
		}

		// Extract variables from stack level
		if stackVars, ok := v2["vars"].(map[string]any); ok {
			for k, v := range stackVars {
				if v != nil {
					stackInfo["vars"].(map[string]any)[k] = v
				}
			}
		}

		// Extract variables from components
		if components, ok := v2["components"].(map[string]any); ok {
			// Helper function to extract vars from component section
			extractComponentVars := func(componentSection map[string]any) {
				for _, comp := range componentSection {
					if compMap, ok := comp.(map[string]any); ok {
						if vars, ok := compMap["vars"].(map[string]any); ok {
							for k, v := range vars {
								if _, exists := stackInfo["vars"].(map[string]any)[k]; !exists && v != nil {
									stackInfo["vars"].(map[string]any)[k] = v
								}
							}
						}
					}
				}
			}

			// Process terraform and helmfile components
			for _, section := range components {
				if sectionMap, ok := section.(map[string]any); ok {
					extractComponentVars(sectionMap)
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
		for k, v := range v2 {
			if k != "vars" && k != "components" {
				stackInfo[k] = v
			}
		}

		return stackInfo
	}

	// Helper function to check if stack has component
	hasComponent := func(components map[string]any, targetComponent string) bool {
		for _, section := range components {
			if compSection, ok := section.(map[string]any); ok {
				if _, exists := compSection[targetComponent]; exists {
					return true
				}
			}
		}
		return false
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
			if components, ok := v2["components"].(map[string]any); ok && hasComponent(components, component) {
				stackInfo := createStackInfo(stackName, v2)
				filteredStacks = append(filteredStacks, stackInfo)
			}
		} else {
			// Include all stacks when no component filter is specified
			stackInfo := createStackInfo(stackName, v2)
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

		// Helper function to check if a column has any non-empty values
		hasValues := func(col schema.ListColumnConfig) bool {
			// Stack is always shown
			if col.Name == "Stack" {
				return true
			}

			tmpl, err := template.New(col.Name).Parse(col.Value)
			if err != nil {
				return false
			}

			// Check if any stack has a non-empty value for this column
			hasNonEmptyValue := false
			for _, stack := range filteredStacks {
				var buf strings.Builder
				if err := tmpl.Execute(&buf, stack); err != nil {
					continue
				}
				value := buf.String()
				if value != "" && value != "<nil>" && value != "<no value>" {
					hasNonEmptyValue = true
					break
				}
			}
			return hasNonEmptyValue
		}

		// Filter out columns with no values
		var activeColumns []schema.ListColumnConfig
		for _, col := range allColumns {
			if hasValues(col) {
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
		// Add custom template functions
		funcMap := template.FuncMap{
			"getVar": func(vars map[string]any, key string) string {
				if val, ok := vars[key]; ok && val != nil {
					return fmt.Sprintf("%v", val)
				}
				return ""
			},
		}

		tmpl, err := template.New(col.Name).Funcs(funcMap).Parse(col.Value)
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
		// Only include columns that have values
		var nonEmptyHeaders []string
		var nonEmptyColumnIndexes []int

		// Find columns that have at least one non-empty value
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

		// Set appropriate delimiter based on format
		fileDelimiter := delimiter
		if delimiter == "\t" || delimiter == "" {
			switch format {
			case FormatCSV:
				fileDelimiter = ","
			case FormatTSV:
				fileDelimiter = "\t"
			}
		}

		var output strings.Builder

		switch format {
		case FormatCSV:
			// Use encoding/csv for proper CSV handling
			writer := csv.NewWriter(&output)
			writer.Comma = rune(fileDelimiter[0])

			// Write headers
			if err := writer.Write(nonEmptyHeaders); err != nil {
				return "", fmt.Errorf("error writing CSV headers: %w", err)
			}

			// Write rows
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
		// If format is empty or "table", use table format
		if format == "" && exec.CheckTTYSupport() {
			// Create a styled table for TTY
			t := table.New().
				Border(lipgloss.ThickBorder()).
				BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorBorder))).
				StyleFunc(func(row, col int) lipgloss.Style {
					style := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
					if row == -1 {
						// Apply CommandName style to all header cells
						return style.Inherit(theme.Styles.CommandName)
					}
					return style.Inherit(theme.Styles.Description)
				}).
				Headers(headers...).
				Rows(rows...)

			return t.String() + utils.GetLineEnding(), nil
		}

		// For non-TTY or when format is explicitly "table", use consistent tabular format
		// that matches the column configuration of the TTY output
		var output strings.Builder

		// Add a separator line after headers for better readability
		headerLine := make([]string, len(headers))
		for i := range headers {
			headerLine[i] = strings.Repeat("-", len(headers[i]))
		}

		output.WriteString(strings.Join(headers, delimiter) + utils.GetLineEnding())
		output.WriteString(strings.Join(headerLine, delimiter) + utils.GetLineEnding())

		for _, row := range rows {
			output.WriteString(strings.Join(row, delimiter) + utils.GetLineEnding())
		}
		return output.String(), nil
	}

}
