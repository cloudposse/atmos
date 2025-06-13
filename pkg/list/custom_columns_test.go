package list

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCustomColumnsFeature tests the custom columns feature across all list commands
func TestCustomColumnsFeature(t *testing.T) {
	tests := []struct {
		name        string
		commandType string
		listConfig  schema.ListConfig
		validate    func(t *testing.T, columns []schema.ListColumnConfig)
	}{
		{
			name:        "vendor command with custom columns",
			commandType: "vendor",
			listConfig: schema.ListConfig{
				Columns: []schema.ListColumnConfig{
					{Name: "Component", Value: "{{ .component_name }}"},
					{Name: "Type", Value: "{{ .component_type }}"},
					{Name: "Path", Value: "{{ .component_path }}"},
				},
			},
			validate: func(t *testing.T, columns []schema.ListColumnConfig) {
				assert.Len(t, columns, 3)
				assert.Equal(t, "Component", columns[0].Name)
				assert.Equal(t, "{{ .component_name }}", columns[0].Value)
			},
		},
		{
			name:        "workflows command with custom columns",
			commandType: "workflows",
			listConfig: schema.ListConfig{
				Columns: []schema.ListColumnConfig{
					{Name: "Workflow", Value: "{{ .workflow_name }}"},
					{Name: "File", Value: "{{ .workflow_file }}"},
					{Name: "Steps", Value: "{{ .workflow_steps }}"},
				},
			},
			validate: func(t *testing.T, columns []schema.ListColumnConfig) {
				assert.Len(t, columns, 3)
				assert.Equal(t, "Workflow", columns[0].Name)
			},
		},
		{
			name:        "components command with custom columns",
			commandType: "components",
			listConfig: schema.ListConfig{
				Columns: []schema.ListColumnConfig{
					{Name: "Name", Value: "{{ .component_name }}"},
					{Name: "Type", Value: "{{ .component_type }}"},
					{Name: "Path", Value: "{{ .component_path }}"},
				},
			},
			validate: func(t *testing.T, columns []schema.ListColumnConfig) {
				assert.Len(t, columns, 3)
				assert.Equal(t, "Name", columns[0].Name)
			},
		},
		{
			name:        "stacks command with custom columns",
			commandType: "stacks",
			listConfig: schema.ListConfig{
				Columns: []schema.ListColumnConfig{
					{Name: "Stack", Value: "{{ .stack_name }}"},
					{Name: "Path", Value: "{{ .stack_path }}"},
				},
			},
			validate: func(t *testing.T, columns []schema.ListColumnConfig) {
				assert.Len(t, columns, 2)
				assert.Equal(t, "Stack", columns[0].Name)
			},
		},
		{
			name:        "values command with custom columns",
			commandType: "values",
			listConfig: schema.ListConfig{
				Columns: []schema.ListColumnConfig{
					{Name: "Stack", Value: "{{ .stack_name }}"},
					{Name: "Key", Value: "{{ .key }}"},
					{Name: "Value", Value: "{{ .value }}"},
				},
			},
			validate: func(t *testing.T, columns []schema.ListColumnConfig) {
				assert.Len(t, columns, 3)
				assert.Equal(t, "Stack", columns[0].Name)
			},
		},
		{
			name:        "vars command with custom columns",
			commandType: "vars",
			listConfig: schema.ListConfig{
				Columns: []schema.ListColumnConfig{
					{Name: "Stack", Value: "{{ .stack_name }}"},
					{Name: "Variable", Value: "{{ .key }}"},
					{Name: "Current Value", Value: "{{ .value }}"},
				},
			},
			validate: func(t *testing.T, columns []schema.ListColumnConfig) {
				assert.Len(t, columns, 3)
				assert.Equal(t, "Variable", columns[1].Name)
			},
		},
		{
			name:        "settings command with custom columns",
			commandType: "settings",
			listConfig: schema.ListConfig{
				Columns: []schema.ListColumnConfig{
					{Name: "Stack", Value: "{{ .stack_name }}"},
					{Name: "Setting", Value: "{{ .key }}"},
					{Name: "Value", Value: "{{ .value }}"},
				},
			},
			validate: func(t *testing.T, columns []schema.ListColumnConfig) {
				assert.Len(t, columns, 3)
				assert.Equal(t, "Setting", columns[1].Name)
			},
		},
		{
			name:        "metadata command with custom columns",
			commandType: "metadata",
			listConfig: schema.ListConfig{
				Columns: []schema.ListColumnConfig{
					{Name: "Stack", Value: "{{ .stack_name }}"},
					{Name: "Key", Value: "{{ .key }}"},
					{Name: "Value", Value: "{{ .value }}"},
				},
			},
			validate: func(t *testing.T, columns []schema.ListColumnConfig) {
				assert.Len(t, columns, 3)
				assert.Equal(t, "Key", columns[1].Name)
			},
		},
		{
			name:        "empty columns - should use defaults",
			commandType: "vendor",
			listConfig:  schema.ListConfig{},
			validate: func(t *testing.T, columns []schema.ListColumnConfig) {
				assert.Greater(t, len(columns), 0)
				// Should have default columns
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Get columns with defaults
			columns := GetColumnsWithDefaults(tt.listConfig.Columns, tt.commandType)
			
			if tt.validate != nil {
				tt.validate(t, columns)
			}
		})
	}
}

// TestTemplateProcessing tests the template processing functionality
func TestTemplateProcessing(t *testing.T) {
	data := map[string]interface{}{
		"stack_name":      "dev-stack",
		"component_name":  "vpc",
		"component_type":  "terraform",
		"key":            "environment",
		"value":          "development",
		"nested": map[string]interface{}{
			"field": "nested-value",
		},
	}

	tests := []struct {
		name     string
		template string
		expected string
		wantErr  bool
	}{
		{
			name:     "simple field",
			template: "{{ .stack_name }}",
			expected: "dev-stack",
		},
		{
			name:     "multiple fields",
			template: "{{ .component_type }}/{{ .component_name }}",
			expected: "terraform/vpc",
		},
		{
			name:     "static text with template",
			template: "Component {{ .component_name }} in stack {{ .stack_name }}",
			expected: "Component vpc in stack dev-stack",
		},
		{
			name:     "missing field",
			template: "{{ .missing_field }}",
			expected: "<no value>",
		},
		{
			name:     "nested field",
			template: "{{ .nested.field }}",
			expected: "nested-value",
		},
		{
			name:     "no template",
			template: "static value",
			expected: "static value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ProcessColumnTemplate(tt.template, data)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
