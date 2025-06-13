package list

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
)

func TestProcessColumnTemplate(t *testing.T) {
	testCases := []struct {
		name          string
		templateValue string
		data          interface{}
		expected      string
		wantErr       bool
	}{
		{
			name:          "simple template",
			templateValue: "{{ .name }}",
			data:          map[string]interface{}{"name": "test-component"},
			expected:      "test-component",
			wantErr:       false,
		},
		{
			name:          "non-template value",
			templateValue: "static value",
			data:          nil,
			expected:      "static value",
			wantErr:       false,
		},
		{
			name:          "complex template",
			templateValue: "{{ .type }}/{{ .name }}",
			data: map[string]interface{}{
				"name": "vpc",
				"type": "terraform",
			},
			expected: "terraform/vpc",
			wantErr:  false,
		},
		{
			name:          "missing field in template",
			templateValue: "{{ .missing }}",
			data:          map[string]interface{}{"name": "test"},
			expected:      "<no value>",
			wantErr:       false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ProcessColumnTemplate(tc.templateValue, tc.data)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestProcessCustomColumns(t *testing.T) {
	columns := []schema.ListColumnConfig{
		{Name: "Component", Value: "{{ .component_name }}"},
		{Name: "Type", Value: "{{ .component_type }}"},
		{Name: "Static", Value: "static-value"},
	}

	data := map[string]interface{}{
		"component_name": "vpc",
		"component_type": "terraform",
	}

	result, err := ProcessCustomColumns(columns, data)
	assert.NoError(t, err)
	assert.Equal(t, "vpc", result["Component"])
	assert.Equal(t, "terraform", result["Type"])
	assert.Equal(t, "static-value", result["Static"])
}

func TestGetDefaultColumns(t *testing.T) {
	testCases := []struct {
		commandType    string
		expectedLength int
		firstColumn    string
	}{
		{"vendor", 4, "Component"},
		{"workflows", 3, "File"},
		{"components", 3, "Component"},
		{"stacks", 2, "Stack"},
		{"values", 3, "Stack"},
		{"vars", 3, "Stack"},
		{"unknown", 0, ""},
	}

	for _, tc := range testCases {
		t.Run(tc.commandType, func(t *testing.T) {
			columns := GetDefaultColumns(tc.commandType)
			assert.Len(t, columns, tc.expectedLength)
			if tc.expectedLength > 0 {
				assert.Equal(t, tc.firstColumn, columns[0].Name)
			}
		})
	}
}

func TestGetColumnsWithDefaults(t *testing.T) {
	customColumns := []schema.ListColumnConfig{
		{Name: "Custom", Value: "{{ .custom }}"},
	}

	// Test with custom columns
	result := GetColumnsWithDefaults(customColumns, "vendor")
	assert.Len(t, result, 1)
	assert.Equal(t, "Custom", result[0].Name)

	// Test with empty custom columns (should return defaults)
	result = GetColumnsWithDefaults([]schema.ListColumnConfig{}, "vendor")
	assert.Len(t, result, 4)
	assert.Equal(t, "Component", result[0].Name)
}

func TestExtractHeaders(t *testing.T) {
	columns := []schema.ListColumnConfig{
		{Name: "Header1", Value: "value1"},
		{Name: "Header2", Value: "value2"},
		{Name: "Header3", Value: "value3"},
	}

	headers := ExtractHeaders(columns)
	assert.Equal(t, []string{"Header1", "Header2", "Header3"}, headers)
}