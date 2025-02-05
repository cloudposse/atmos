package list

import (
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
		}
		// Copy all stack configuration to allow full access in templates
		for k, v := range v2 {
			stackInfo[k] = v
		}
		return stackInfo
	}

	var filteredStacks []map[string]any

	if component != "" {
		// Filter stacks by component
		for stackName, stackData := range stacksMap {
			v2, ok := stackData.(map[string]any)
			if !ok {
				continue
			}
			// Check if the component exists in any component type section
			if components, ok := v2["components"].(map[string]any); ok {
				componentFound := false
				for _, componentSection := range components {
					if compSection, ok := componentSection.(map[string]any); ok {
						if _, exists := compSection[component]; exists {
							componentFound = true
							break
						}
					}
				}
				if componentFound {
					stackInfo := createStackInfo(stackName, v2)
					filteredStacks = append(filteredStacks, stackInfo)
				}
			}
		}
	} else {
		// List all stacks
		for stackName, stackData := range stacksMap {
			v2, ok := stackData.(map[string]any)
			if !ok {
				continue
			}
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
		listConfig.Columns = []schema.ListColumnConfig{
			{Name: "Stack", Value: "{{ .atmos_stack }}"},
			{Name: "File", Value: "{{ .atmos_stack_file }}"},
		}
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
			row[j] = buf.String()
		}
		rows[i] = row
	}

	// Handle different output formats
	switch format {
	case FormatJSON:
		// Convert to JSON format using dynamic fields
		var stacks []map[string]string
		for _, row := range rows {
			s := make(map[string]string)
			for i, header := range headers {
				// Convert header to lowercase for consistent JSON field names
				s[strings.ToLower(header)] = row[i]
			}
			stacks = append(stacks, s)
		}
		jsonBytes, err := json.MarshalIndent(stacks, "", "  ")
		if err != nil {
			return "", fmt.Errorf("error formatting JSON output: %w", err)
		}
		return string(jsonBytes), nil

	case FormatCSV:
		var output strings.Builder
		output.WriteString(strings.Join(headers, delimiter) + utils.GetLineEnding())
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
					if row == 0 {
						// Apply CommandName style to all header cells
						return style.Inherit(theme.Styles.CommandName)
					}
					return style.Inherit(theme.Styles.Description)
				}).
				Headers(headers...).
				Rows(rows...)

			return t.String() + utils.GetLineEnding(), nil
		}

		// Default to simple tabular format for non-TTY or when format is explicitly "table"
		var output strings.Builder
		output.WriteString(strings.Join(headers, delimiter) + utils.GetLineEnding())
		for _, row := range rows {
			output.WriteString(strings.Join(row, delimiter) + utils.GetLineEnding())
		}
		return output.String(), nil
	}
}
