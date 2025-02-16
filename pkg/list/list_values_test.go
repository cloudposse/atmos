package list

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterAndListValues(t *testing.T) {
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
							"region":      "us-east-1",
							"cidr_block":  "10.1.0.0/16",
						},
					},
				},
			},
		},
		"prod": map[string]interface{}{
			"components": map[string]interface{}{
				"terraform": map[string]interface{}{
					"vpc": map[string]interface{}{
						"abstract": true,
						"vars": map[string]interface{}{
							"environment": "prod",
							"region":      "us-east-1",
							"cidr_block":  "10.2.0.0/16",
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name            string
		component       string
		query           string
		includeAbstract bool
		maxColumns      int
		format          string
		delimiter       string
		expectError     bool
		expectedError   string
		checkFunc       func(t *testing.T, output string)
	}{
		{
			name:      "basic table format",
			component: "vpc",
			format:    "",
			checkFunc: func(t *testing.T, output string) {
				assert.Contains(t, output, "environment")
				assert.Contains(t, output, "region")
				assert.Contains(t, output, "cidr_block")
				assert.Contains(t, output, "dev")
				assert.Contains(t, output, "staging")
				assert.NotContains(t, output, "prod") // Abstract component
			},
		},
		{
			name:            "include abstract components",
			component:       "vpc",
			includeAbstract: true,
			format:          "",
			checkFunc: func(t *testing.T, output string) {
				assert.Contains(t, output, "prod")
			},
		},
		{
			name:      "json format",
			component: "vpc",
			format:    "json",
			checkFunc: func(t *testing.T, output string) {
				var result map[string]interface{}
				err := json.Unmarshal([]byte(output), &result)
				assert.NoError(t, err)
				assert.Contains(t, result, "dev")
				assert.Contains(t, result, "staging")
			},
		},
		{
			name:      "csv format",
			component: "vpc",
			format:    "csv",
			delimiter: ",",
			checkFunc: func(t *testing.T, output string) {
				assert.Contains(t, output, "Key,dev,staging")
				assert.Contains(t, output, "environment,dev,staging")
			},
		},
		{
			name:      "query filter",
			component: "vpc",
			query:     "environment",
			format:    "json",
			checkFunc: func(t *testing.T, output string) {
				var result map[string]interface{}
				err := json.Unmarshal([]byte(output), &result)
				assert.NoError(t, err)
				// Check that we have the expected values
				assert.Equal(t, "dev", result["dev"])
				assert.Equal(t, "staging", result["staging"])
			},
		},
		{
			name:       "max columns",
			component:  "vpc",
			maxColumns: 1,
			format:     "",
			checkFunc: func(t *testing.T, output string) {
				assert.Contains(t, output, "dev")
				assert.NotContains(t, output, "staging")
			},
		},
		{
			name:        "invalid format",
			component:   "vpc",
			format:      "invalid",
			expectError: true,
		},
		{
			name:          "component not found",
			component:     "nonexistent",
			expectError:   true,
			expectedError: "no values found for component 'nonexistent'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := FilterAndListValues(stacksMap, tt.component, tt.query, tt.includeAbstract, tt.maxColumns, tt.format, tt.delimiter)

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedError != "" {
					assert.Equal(t, tt.expectedError, err.Error())
				}
				return
			}

			require.NoError(t, err)
			if tt.checkFunc != nil {
				tt.checkFunc(t, output)
			}
		})
	}
}
