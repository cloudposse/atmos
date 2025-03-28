package list

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterAndListValues(t *testing.T) {
	// Mock stacks data
	stacksMap := map[string]interface{}{
		"staging": map[string]interface{}{
			"components": map[string]interface{}{
				"terraform": map[string]interface{}{
					"vpc": map[string]interface{}{
						"vars": map[string]interface{}{
							"environment": "staging",
							"region":      "us-east-1",
							"cidr_block":  "10.1.0.0/16",
							"tags": map[string]interface{}{
								"Environment": "staging",
								"Team":        "devops",
							},
							"subnets": []interface{}{
								"10.1.1.0/24",
								"10.1.2.0/24",
							},
						},
					},
				},
			},
			"settings": map[string]interface{}{
				"environment": map[string]interface{}{
					"name":   "staging",
					"region": "us-east-1",
				},
			},
			"metadata": map[string]interface{}{
				"team":    "platform",
				"version": "1.0.0",
			},
		},
		"dev": map[string]interface{}{
			"components": map[string]interface{}{
				"terraform": map[string]interface{}{
					"vpc": map[string]interface{}{
						"vars": map[string]interface{}{
							"environment": "dev",
							"region":      "us-east-1",
							"cidr_block":  "10.0.0.0/16",
							"tags": map[string]interface{}{
								"Environment": "dev",
								"Team":        "devops",
							},
							"subnets": []interface{}{
								"10.0.1.0/24",
								"10.0.2.0/24",
							},
						},
					},
				},
			},
			"settings": map[string]interface{}{
				"environment": map[string]interface{}{
					"name":   "dev",
					"region": "us-east-1",
				},
			},
			"metadata": map[string]interface{}{
				"team":    "platform",
				"version": "1.0.0",
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
							"tags": map[string]interface{}{
								"Environment": "prod",
								"Team":        "devops",
							},
							"subnets": []interface{}{
								"10.2.1.0/24",
								"10.2.2.0/24",
							},
						},
					},
				},
			},
			"settings": map[string]interface{}{
				"environment": map[string]interface{}{
					"name":   "prod",
					"region": "us-east-1",
				},
			},
			"metadata": map[string]interface{}{
				"team":    "platform",
				"version": "1.0.0",
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
		stackPattern    string
		expectError     bool
		expectedError   string
		checkFunc       func(t *testing.T, output string)
	}{
		{
			name:         "basic table format",
			component:    "vpc",
			format:       "",
			stackPattern: "dev,staging",
			checkFunc: func(t *testing.T, output string) {
				assert.Contains(t, output, "environment")
				assert.Contains(t, output, "region")
				assert.Contains(t, output, "cidr_block")
				assert.Contains(t, output, "dev")
				assert.Contains(t, output, "staging")
				assert.NotContains(t, output, "prod")
			},
		},
		{
			name:            "include abstract components",
			component:       "vpc",
			includeAbstract: true,
			format:          "json", // Changed to JSON to avoid terminal width issues
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
			name:      "yaml format",
			component: "vpc",
			format:    "yaml",
			checkFunc: func(t *testing.T, output string) {
				// YAML format should contain the environment values
				assert.Contains(t, output, "dev:")
				assert.Contains(t, output, "staging:")
				assert.Contains(t, output, "environment: dev")
				assert.Contains(t, output, "environment: staging")
				assert.Contains(t, output, "cidr_block:")
			},
		},
		{
			name:         "csv format",
			component:    "vpc",
			format:       "csv",
			delimiter:    ",",
			stackPattern: "dev,staging",
			checkFunc: func(t *testing.T, output string) {
				assert.Contains(t, output, "Key,dev,staging")
				assert.Contains(t, output, "environment,dev,staging")
			},
		},
		{
			name:         "tsv format",
			component:    "vpc",
			format:       "tsv",
			delimiter:    "\t",
			stackPattern: "dev,staging",
			checkFunc: func(t *testing.T, output string) {
				assert.Contains(t, output, "Key\tdev\tstaging")
				assert.Contains(t, output, "environment\tdev\tstaging")
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
		{
			name:         "stack pattern matching",
			component:    "vpc",
			stackPattern: "dev*",
			format:       "json",
			checkFunc: func(t *testing.T, output string) {
				var result map[string]interface{}
				err := json.Unmarshal([]byte(output), &result)
				assert.NoError(t, err)
				assert.Contains(t, result, "dev")
				assert.NotContains(t, result, "staging")
				assert.NotContains(t, result, "prod")
			},
		},
		{
			name:      "settings component",
			component: "settings",
			format:    "json",
			checkFunc: func(t *testing.T, output string) {
				var result map[string]interface{}
				err := json.Unmarshal([]byte(output), &result)
				assert.NoError(t, err)
				for _, env := range []string{"dev", "staging", "prod"} {
					envData, ok := result[env].(map[string]interface{})
					assert.True(t, ok)
					envSettings, ok := envData["environment"].(map[string]interface{})
					assert.True(t, ok)
					assert.Contains(t, envSettings, "name")
					assert.Contains(t, envSettings, "region")
				}
			},
		},
		{
			name:      "metadata component",
			component: "metadata",
			format:    "json",
			checkFunc: func(t *testing.T, output string) {
				var result map[string]interface{}
				err := json.Unmarshal([]byte(output), &result)
				assert.NoError(t, err)
				for _, env := range []string{"dev", "staging", "prod"} {
					envData, ok := result[env].(map[string]interface{})
					assert.True(t, ok)
					assert.Equal(t, "platform", envData["team"])
					assert.Equal(t, "1.0.0", envData["version"])
				}
			},
		},
		{
			name:      "query filtering - nested map",
			component: "vpc",
			query:     ".tags",
			format:    "json",
			checkFunc: func(t *testing.T, output string) {
				var result map[string]interface{}
				err := json.Unmarshal([]byte(output), &result)
				assert.NoError(t, err)
				for _, env := range []string{"dev", "staging"} {
					// After removing the value wrapper, the tags map is directly under each env
					tagsMap, ok := result[env].(map[string]interface{})
					assert.True(t, ok)
					assert.Equal(t, "devops", tagsMap["Team"])
				}
			},
		},
		{
			name:      "query filtering - array",
			component: "vpc",
			query:     ".subnets",
			format:    "json",
			checkFunc: func(t *testing.T, output string) {
				var result map[string]interface{}
				err := json.Unmarshal([]byte(output), &result)
				assert.NoError(t, err)
				// After removing the value wrapper, the subnets array is directly under dev
				subnets, ok := result["dev"].([]interface{})
				assert.True(t, ok)
				// Convert to strings for easier assertion
				subnetStrings := make([]string, len(subnets))
				for i, subnet := range subnets {
					subnetStrings[i] = fmt.Sprintf("%v", subnet)
				}
				assert.Contains(t, subnetStrings, "10.0.1.0/24")
				assert.Contains(t, subnetStrings, "10.0.2.0/24")
			},
		},
		{
			name:      "settings with query",
			component: "settings",
			query:     ".environment.name",
			format:    "json",
			checkFunc: func(t *testing.T, output string) {
				var result map[string]interface{}
				err := json.Unmarshal([]byte(output), &result)
				assert.NoError(t, err)
				for env, expected := range map[string]string{"dev": "dev", "staging": "staging", "prod": "prod"} {
					name, ok := result[env].(string)
					assert.True(t, ok)
					assert.Equal(t, expected, name)
				}
			},
		},
		{
			name:      "metadata with query",
			component: "metadata",
			query:     ".team",
			format:    "json",
			checkFunc: func(t *testing.T, output string) {
				var result map[string]interface{}
				err := json.Unmarshal([]byte(output), &result)
				assert.NoError(t, err)
				// After removing the value wrapper, the team value is directly under each env
				for _, env := range []string{"dev", "staging", "prod"} {
					team, ok := result[env].(string)
					assert.True(t, ok)
					assert.Equal(t, "platform", team)
				}
			},
		},
		{
			name:      "scalar query result - table format",
			component: "vpc",
			query:     ".region",
			format:    "table",
			checkFunc: func(t *testing.T, output string) {
				assert.Contains(t, output, "Key")
				assert.Contains(t, output, "value")
				assert.Contains(t, output, "us-east-1")
			},
		},
		{
			name:      "scalar query result - csv format",
			component: "vpc",
			query:     ".region",
			format:    "csv",
			checkFunc: func(t *testing.T, output string) {
				assert.Contains(t, output, "Key,dev,prod,staging")
				assert.Contains(t, output, "value,us-east-1,us-east-1,us-east-1")
			},
		},
		{
			name:      "array query result - yaml format",
			component: "vpc",
			query:     ".subnets",
			format:    "yaml",
			checkFunc: func(t *testing.T, output string) {
				assert.Contains(t, output, "dev:")
				assert.Contains(t, output, "- 10.0.1.0/24")
				assert.Contains(t, output, "- 10.0.2.0/24")
				assert.Contains(t, output, "staging:")
				assert.Contains(t, output, "- 10.1.1.0/24")
				assert.Contains(t, output, "- 10.1.2.0/24")
			},
		},
		{
			name:      "array query result - tsv format",
			component: "vpc",
			query:     ".subnets",
			format:    "tsv",
			checkFunc: func(t *testing.T, output string) {
				assert.Contains(t, output, "Key\tdev\tprod\tstaging")
				assert.Contains(t, output, "value\t")
				assert.Contains(t, output, "10.0.1.0/24,10.0.2.0/24")
				assert.Contains(t, output, "10.2.1.0/24,10.2.2.0/24") // prod stack subnets
				assert.Contains(t, output, "10.1.1.0/24,10.1.2.0/24")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := FilterAndListValues(stacksMap, &FilterOptions{
				Component:       tt.component,
				Query:           tt.query,
				IncludeAbstract: tt.includeAbstract,
				MaxColumns:      tt.maxColumns,
				FormatStr:       tt.format,
				Delimiter:       tt.delimiter,
				StackPattern:    tt.stackPattern,
			})

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
