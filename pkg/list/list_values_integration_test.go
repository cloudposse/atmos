package list

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFilterAndListValuesBackwardCompatibility ensures that the function works correctly
// when no custom columns are configured (backward compatibility)
func TestFilterAndListValuesBackwardCompatibility(t *testing.T) {
	// Mock stacks data
	stacksMap := map[string]interface{}{
		"dev": map[string]interface{}{
			"components": map[string]interface{}{
				"terraform": map[string]interface{}{
					"vpc": map[string]interface{}{
						"vars": map[string]interface{}{
							"environment": "dev",
							"region":      "us-east-1",
							"cidr_block":  "10.0.0.0/16",
						},
					},
				},
			},
		},
		"staging": map[string]interface{}{
			"components": map[string]interface{}{
				"terraform": map[string]interface{}{
					"vpc": map[string]interface{}{
						"vars": map[string]interface{}{
							"environment": "staging",
							"region":      "us-east-2",
							"cidr_block":  "10.1.0.0/16",
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name        string
		filterOpts  *FilterOptions
		expectError bool
		checkFunc   func(t *testing.T, output string)
	}{
		{
			name: "backward compatibility - table format",
			filterOpts: &FilterOptions{
				Component: "vpc",
				FormatStr: "table",
			},
			checkFunc: func(t *testing.T, output string) {
				// Should use default columns
				assert.Contains(t, output, "Stack")
				assert.Contains(t, output, "Key")
				assert.Contains(t, output, "Value")
				assert.Contains(t, output, "dev")
				assert.Contains(t, output, "staging")
				assert.Contains(t, output, "environment")
				assert.Contains(t, output, "region")
				assert.Contains(t, output, "cidr_block")
			},
		},
		{
			name: "backward compatibility - json format",
			filterOpts: &FilterOptions{
				Component: "vpc",
				FormatStr: "json",
			},
			checkFunc: func(t *testing.T, output string) {
				assert.Contains(t, output, "dev")
				assert.Contains(t, output, "staging")
				assert.Contains(t, output, "environment")
				assert.Contains(t, output, "region")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call with empty ListConfig to simulate no custom columns
			output, err := FilterAndListValuesWithColumns(stacksMap, tt.filterOpts, schema.ListConfig{})

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			if tt.checkFunc != nil {
				tt.checkFunc(t, output)
			}
		})
	}
}

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