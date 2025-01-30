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

	var filteredStacks []map[string]any

	if component != "" {
		// Filter stacks by component
		for stackName, stackData := range stacksMap {
			v2, ok := stackData.(map[string]any)
			if !ok {
				continue
			}
			components, ok := v2["components"].(map[string]any)
			if !ok {
				continue
			}
			terraform, ok := components["terraform"].(map[string]any)
			if !ok {
				continue
			}
			if _, exists := terraform[component]; exists {
				stackInfo := map[string]any{
					"atmos_stack": stackName,
					"vars":        v2["vars"],
				}

				// Safely get tenant and environment from vars
				if vars, ok := v2["vars"].(map[string]any); ok {
					tenant, _ := vars["tenant"].(string)
					environment, _ := vars["environment"].(string)
					stage, _ := vars["stage"].(string)
					stackInfo["atmos_stack_file"] = fmt.Sprintf("orgs/acme/%s/%s/%s", tenant, environment, stage)
				} else {
					stackInfo["atmos_stack_file"] = fmt.Sprintf("stacks/deploy/%s.yaml", stackName)
				}

				// Add component vars if they exist
				if components, ok := v2["components"].(map[string]any); ok {
					if terraform, ok := components["terraform"].(map[string]any); ok {
						for _, comp := range terraform {
							if compSection, ok := comp.(map[string]any); ok {
								if compVars, ok := compSection["vars"].(map[string]any); ok {
									// Merge component vars with stack vars
									if stackInfo["vars"] == nil {
										stackInfo["vars"] = make(map[string]any)
									}
									for k, v := range compVars {
										stackInfo["vars"].(map[string]any)[k] = v
									}
								}
							}
						}
					}
				}
				filteredStacks = append(filteredStacks, stackInfo)
			}
		}
	} else {
		// List all stacks
		for stackName, stackData := range stacksMap {
			v2, ok := stackData.(map[string]any)
			if !ok {
				continue
			}
			stackInfo := map[string]any{
				"atmos_stack": stackName,
				"vars":        v2["vars"],
			}

			// Safely get tenant and environment from vars
			if vars, ok := v2["vars"].(map[string]any); ok {
				tenant, _ := vars["tenant"].(string)
				environment, _ := vars["environment"].(string)
				stage, _ := vars["stage"].(string)
				stackInfo["atmos_stack_file"] = fmt.Sprintf("orgs/acme/%s/%s/%s", tenant, environment, stage)
			} else {
				stackInfo["atmos_stack_file"] = fmt.Sprintf("stacks/deploy/%s.yaml", stackName)
			}

			// Add component vars if they exist
			if components, ok := v2["components"].(map[string]any); ok {
				if terraform, ok := components["terraform"].(map[string]any); ok {
					for _, comp := range terraform {
						if compSection, ok := comp.(map[string]any); ok {
							if compVars, ok := compSection["vars"].(map[string]any); ok {
								// Merge component vars with stack vars
								if stackInfo["vars"] == nil {
									stackInfo["vars"] = make(map[string]any)
								}
								for k, v := range compVars {
									stackInfo["vars"].(map[string]any)[k] = v
								}
							}
						}
					}
				}
			}
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

	// Process each stack and populate rows
	for i, stack := range filteredStacks {
		row := make([]string, len(listConfig.Columns))
		for j, col := range listConfig.Columns {
			tmpl, err := template.New("column").Parse(col.Value)
			if err != nil {
				return "", fmt.Errorf("error parsing template for column %s: %w", col.Name, err)
			}

			var buf strings.Builder
			if err := tmpl.Execute(&buf, stack); err != nil {
				return "", fmt.Errorf("error executing template for column %s: %w", col.Name, err)
			}
			row[j] = buf.String()
		}
		rows[i] = row
	}

	// Handle different output formats
	switch format {
	case FormatJSON:
		var result []map[string]string
		for _, row := range rows {
			item := make(map[string]string)
			for i, header := range headers {
				item[header] = row[i]
			}
			result = append(result, item)
		}
		jsonBytes, err := json.MarshalIndent(result, "", "  ")
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
		// Check for TTY support
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

		var output strings.Builder
		// Write headers
		headerRow := make([]string, len(headers))
		for i, h := range headers {
			headerRow[i] = h
		}
		output.WriteString(strings.Join(headerRow, "\t") + utils.GetLineEnding())

		// Write rows
		for _, row := range rows {
			output.WriteString(strings.Join(row, "\t") + utils.GetLineEnding())
		}
		return output.String(), nil
	}
}
