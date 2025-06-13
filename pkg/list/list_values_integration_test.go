package list

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCustomColumnsWithTemplates tests custom columns with template processing
func TestCustomColumnsWithTemplates(t *testing.T) {
	stacksMap := map[string]interface{}{
		"dev": map[string]interface{}{
			"components": map[string]interface{}{
				"terraform": map[string]interface{}{
					"vpc": map[string]interface{}{
						"vars": map[string]interface{}{
							"environment": "dev",
							"region":      "us-east-1",
						},
					},
				},
			},
		},
	}

	// Test custom columns with templates
	listConfig := schema.ListConfig{
		Columns: []schema.ListColumnConfig{
			{Name: "Stack", Value: "{{ .stack_name }}"},
			{Name: "Variable", Value: "{{ .key }}"},
			{Name: "Current Value", Value: "{{ .value }}"},
		},
	}

	output, err := FilterAndListValuesWithColumns(stacksMap, &FilterOptions{
		Component: "vpc",
		FormatStr: "table",
	}, listConfig)

	require.NoError(t, err)
	assert.Contains(t, output, "Stack")
	assert.Contains(t, output, "Variable")
	assert.Contains(t, output, "Current Value")
	assert.Contains(t, output, "dev")
	assert.Contains(t, output, "environment")
	assert.Contains(t, output, "region")
}
