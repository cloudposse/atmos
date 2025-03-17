package values

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractStackValues(t *testing.T) {
	tests := []struct {
		name            string
		stacksMap       map[string]interface{}
		component       string
		includeAbstract bool
		expectedValues  map[string]interface{}
		expectError     bool
	}{
		{
			name: "extract regular component values",
			stacksMap: map[string]interface{}{
				"stack1": map[string]interface{}{
					"components": map[string]interface{}{
						"terraform": map[string]interface{}{
							"vpc": map[string]interface{}{
								"vars": map[string]interface{}{
									"region": "us-west-2",
								},
							},
						},
					},
				},
			},
			component: "vpc",
			expectedValues: map[string]interface{}{
				"stack1": map[string]interface{}{
					"region": "us-west-2",
				},
			},
		},
		{
			name: "extract settings component",
			stacksMap: map[string]interface{}{
				"stack1": map[string]interface{}{
					KeySettings: map[string]interface{}{
						"region": "us-west-2",
					},
				},
			},
			component: KeySettings,
			expectedValues: map[string]interface{}{
				"stack1": map[string]interface{}{
					"region": "us-west-2",
				},
			},
		},
		{
			name: "extract metadata component",
			stacksMap: map[string]interface{}{
				"stack1": map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "stack1",
						"type": "terraform",
					},
				},
			},
			component: "metadata",
			expectedValues: map[string]interface{}{
				"stack1": map[string]interface{}{
					"name": "stack1",
					"type": "terraform",
				},
			},
		},
		{
			name: "skip abstract component",
			stacksMap: map[string]interface{}{
				"stack1": map[string]interface{}{
					"components": map[string]interface{}{
						"terraform": map[string]interface{}{
							"vpc": map[string]interface{}{
								"abstract": true,
								"vars": map[string]interface{}{
									"region": "us-west-2",
								},
							},
						},
					},
				},
			},
			component:       "vpc",
			includeAbstract: false,
			expectError:     true,
		},
		{
			name: "include abstract component",
			stacksMap: map[string]interface{}{
				"stack1": map[string]interface{}{
					"components": map[string]interface{}{
						"terraform": map[string]interface{}{
							"vpc": map[string]interface{}{
								"abstract": true,
								"vars": map[string]interface{}{
									"region": "us-west-2",
								},
							},
						},
					},
				},
			},
			component:       "vpc",
			includeAbstract: true,
			expectedValues: map[string]interface{}{
				"stack1": map[string]interface{}{
					"region": "us-west-2",
				},
			},
		},
		{
			name: "no values found",
			stacksMap: map[string]interface{}{
				"stack1": map[string]interface{}{
					"components": map[string]interface{}{},
				},
			},
			component:   "vpc",
			expectError: true,
		},
		{
			name: "component with terraform prefix",
			stacksMap: map[string]interface{}{
				"stack1": map[string]interface{}{
					"components": map[string]interface{}{
						"terraform": map[string]interface{}{
							"vpc": map[string]interface{}{
								"vars": map[string]interface{}{
									"region": "us-west-2",
								},
							},
						},
					},
				},
			},
			component: "terraform/vpc",
			expectedValues: map[string]interface{}{
				"stack1": map[string]interface{}{
					"region": "us-west-2",
				},
			},
		},
		{
			name: "invalid stack data type",
			stacksMap: map[string]interface{}{
				"stack1": "not a map",
			},
			component:   "vpc",
			expectError: true,
		},
		{
			name:        "empty stacks map",
			stacksMap:   map[string]interface{}{},
			component:   "vpc",
			expectError: true,
		},
	}

	extractor := NewDefaultExtractor()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values, err := extractor.ExtractStackValues(test.stacksMap, test.component, test.includeAbstract)

			if test.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, test.expectedValues, values)
		})
	}
}

func TestApplyValueQuery(t *testing.T) {
	tests := []struct {
		name           string
		values         map[string]interface{}
		query          string
		expectedResult map[string]interface{}
		expectError    bool
	}{
		{
			name: "simple query with YQ expression",
			values: map[string]interface{}{
				"stack1": map[string]interface{}{
					"region": "us-west-2",
				},
			},
			query: ".region",
			expectedResult: map[string]interface{}{
				"stack1": "us-west-2",
			},
		},
		{
			name: "nested query with YQ expression",
			values: map[string]interface{}{
				"stack1": map[string]interface{}{
					"vpc": map[string]interface{}{
						"cidr": "10.0.0.0/16",
					},
				},
			},
			query: ".vpc.cidr",
			expectedResult: map[string]interface{}{
				"stack1": "10.0.0.0/16",
			},
		},
		{
			name: "array query with YQ expression",
			values: map[string]interface{}{
				"stack1": map[string]interface{}{
					"subnets": []interface{}{
						"10.0.1.0/24",
						"10.0.2.0/24",
					},
				},
			},
			query: ".subnets[0]",
			expectedResult: map[string]interface{}{
				"stack1": "10.0.1.0/24",
			},
		},
		{
			name: "empty query returns all values",
			values: map[string]interface{}{
				"stack1": map[string]interface{}{
					"region": "us-west-2",
				},
			},
			query: "",
			expectedResult: map[string]interface{}{
				"stack1": map[string]interface{}{
					"region": "us-west-2",
				},
			},
		},
		{
			name: "array result with YQ expression",
			values: map[string]interface{}{
				"stack1": map[string]interface{}{
					"subnets": []interface{}{
						"10.0.1.0/24",
						"10.0.2.0/24",
					},
				},
			},
			query: ".subnets",
			expectedResult: map[string]interface{}{
				"stack1": []interface{}{
					"10.0.1.0/24",
					"10.0.2.0/24",
				},
			},
		},
		{
			name: "YQ filtering expression",
			values: map[string]interface{}{
				"stack1": map[string]interface{}{
					"environments": map[string]interface{}{
						"dev": map[string]interface{}{
							"enabled": true,
						},
						"prod": map[string]interface{}{
							"enabled": false,
						},
					},
				},
			},
			query: ".environments | with_entries(select(.value.enabled == true))",
			expectedResult: map[string]interface{}{
				"stack1": map[string]interface{}{
					"dev": map[string]interface{}{
						"enabled": true,
					},
				},
			},
		},
		{
			name: "invalid YQ expression",
			values: map[string]interface{}{
				"stack1": map[string]interface{}{
					"region": "us-west-2",
				},
			},
			query:       ".[invalid",
			expectError: true,
		},
		{
			name: "no matching values for YQ expression",
			values: map[string]interface{}{
				"stack1": map[string]interface{}{
					"vpc": map[string]interface{}{
						"cidr": "10.0.0.0/16",
					},
				},
			},
			query:       ".nonexistent",
			expectError: true,
		},
	}

	extractor := NewDefaultExtractor()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := extractor.ApplyValueQuery(test.values, test.query)

			if test.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, test.expectedResult, result)
		})
	}
}
