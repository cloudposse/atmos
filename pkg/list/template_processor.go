package list

import (
	"fmt"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
)

func ProcessColumnTemplate(templateValue string, data interface{}) (string, error) {
	isTemplate, err := exec.IsGolangTemplate(templateValue)
	if err != nil {
		return templateValue, nil
	}
	
	if !isTemplate {
		return templateValue, nil
	}

	result, err := exec.ProcessTmpl(fmt.Sprintf("column-template"), templateValue, data, true)
	if err != nil {
		return "", fmt.Errorf("failed to process template '%s': %w", templateValue, err)
	}

	return result, nil
}

func ProcessCustomColumns(columns []schema.ListColumnConfig, data interface{}) (map[string]interface{}, error) {
	row := make(map[string]interface{})

	for _, col := range columns {
		value, err := ProcessColumnTemplate(col.Value, data)
		if err != nil {
			value = col.Value
		}
		row[col.Name] = value
	}

	return row, nil
}

func GetDefaultColumns(commandType string) []schema.ListColumnConfig {
	switch commandType {
	case "vendor":
		return []schema.ListColumnConfig{
			{Name: "Component", Value: "{{ .atmos_component }}"},
			{Name: "Type", Value: "{{ .atmos_vendor_type }}"},
			{Name: "Manifest", Value: "{{ .atmos_vendor_file }}"},
			{Name: "Folder", Value: "{{ .atmos_vendor_target }}"},
		}
	case "workflows":
		return []schema.ListColumnConfig{
			{Name: "File", Value: "{{ .workflow_file }}"},
			{Name: "Workflow", Value: "{{ .workflow_name }}"},
			{Name: "Description", Value: "{{ .workflow_description }}"},
		}
	case "components":
		return []schema.ListColumnConfig{
			{Name: "Component", Value: "{{ .component_name }}"},
			{Name: "Type", Value: "{{ .component_type }}"},
			{Name: "Path", Value: "{{ .component_path }}"},
		}
	case "stacks":
		return []schema.ListColumnConfig{
			{Name: "Stack", Value: "{{ .stack_name }}"},
			{Name: "Path", Value: "{{ .stack_path }}"},
		}
	case "values", "vars", "settings", "metadata":
		return []schema.ListColumnConfig{
			{Name: "Stack", Value: "{{ .stack_name }}"},
			{Name: "Key", Value: "{{ .key }}"},
			{Name: "Value", Value: "{{ .value }}"},
		}
	default:
		return []schema.ListColumnConfig{}
	}
}

func GetColumnsWithDefaults(customColumns []schema.ListColumnConfig, commandType string) []schema.ListColumnConfig {
	if len(customColumns) > 0 {
		return customColumns
	}
	return GetDefaultColumns(commandType)
}

func ExtractHeaders(columns []schema.ListColumnConfig) []string {
	headers := make([]string, len(columns))
	for i, col := range columns {
		headers[i] = col.Name
	}
	return headers
}