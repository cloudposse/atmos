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
					"settings": map[string]interface{}{
						"region": "us-west-2",
					},
				},
			},
			component: "settings",
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
			name: "simple query",
			values: map[string]interface{}{
				"stack1": map[string]interface{}{
					"region": "us-west-2",
				},
			},
			query: "region",
			expectedResult: map[string]interface{}{
				"stack1": map[string]interface{}{
					"value": "us-west-2",
				},
			},
		},
		{
			name: "nested query",
			values: map[string]interface{}{
				"stack1": map[string]interface{}{
					"vpc": map[string]interface{}{
						"cidr": "10.0.0.0/16",
					},
				},
			},
			query: "vpc.cidr",
			expectedResult: map[string]interface{}{
				"stack1": map[string]interface{}{
					"value": "10.0.0.0/16",
				},
			},
		},
		{
			name: "array query",
			values: map[string]interface{}{
				"stack1": map[string]interface{}{
					"subnets": []interface{}{
						"10.0.1.0/24",
						"10.0.2.0/24",
					},
				},
			},
			query: "subnets.0",
			expectedResult: map[string]interface{}{
				"stack1": map[string]interface{}{
					"value": "10.0.1.0/24",
				},
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
			name: "array formatting",
			values: map[string]interface{}{
				"stack1": map[string]interface{}{
					"subnets": []interface{}{
						"10.0.1.0/24",
						"10.0.2.0/24",
					},
				},
			},
			query: "subnets",
			expectedResult: map[string]interface{}{
				"stack1": map[string]interface{}{
					"value": "10.0.1.0/24,10.0.2.0/24",
				},
			},
		},
		{
			name: "invalid query",
			values: map[string]interface{}{
				"stack1": map[string]interface{}{
					"region": "us-west-2",
				},
			},
			query:       "invalid.path",
			expectError: true,
		},
		{
			name: "stack data not a map",
			values: map[string]interface{}{
				"stack1": "not a map",
			},
			query:       "region",
			expectError: true,
		},
		{
			name: "no matching values for query",
			values: map[string]interface{}{
				"stack1": map[string]interface{}{
					"vpc": map[string]interface{}{
						"cidr": "10.0.0.0/16",
					},
				},
			},
			query:       "nonexistent",
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

// Helper function tests

func TestHandleSpecialComponent(t *testing.T) {
	tests := []struct {
		name          string
		stack         map[string]interface{}
		component     string
		expected      map[string]interface{}
		expectedFound bool
	}{
		{
			name: "settings at top level",
			stack: map[string]interface{}{
				"settings": map[string]interface{}{
					"region": "us-west-2",
				},
			},
			component: "settings",
			expected: map[string]interface{}{
				"region": "us-west-2",
			},
			expectedFound: true,
		},
		{
			name: "metadata at top level",
			stack: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "stack1",
				},
			},
			component: "metadata",
			expected: map[string]interface{}{
				"name": "stack1",
			},
			expectedFound: true,
		},
		{
			name: "settings from components",
			stack: map[string]interface{}{
				"components": map[string]interface{}{
					"terraform": map[string]interface{}{
						"vpc": map[string]interface{}{
							"settings": map[string]interface{}{
								"region": "us-west-2",
							},
						},
					},
				},
			},
			component: "settings",
			expected: map[string]interface{}{
				"vpc": map[string]interface{}{
					"region": "us-west-2",
				},
			},
			expectedFound: true,
		},
		{
			name: "component not found",
			stack: map[string]interface{}{
				"components": map[string]interface{}{},
			},
			component:     "nonexistent",
			expected:      nil,
			expectedFound: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, found := handleSpecialComponent(test.stack, test.component)
			assert.Equal(t, test.expectedFound, found)
			if found {
				assert.Equal(t, test.expected, result)
			}
		})
	}
}

func TestExtractSettingsFromComponents(t *testing.T) {
	tests := []struct {
		name          string
		stack         map[string]interface{}
		expected      map[string]interface{}
		expectedFound bool
	}{
		{
			name: "extract settings from terraform components",
			stack: map[string]interface{}{
				"components": map[string]interface{}{
					"terraform": map[string]interface{}{
						"vpc": map[string]interface{}{
							"settings": map[string]interface{}{
								"region": "us-west-2",
							},
						},
						"eks": map[string]interface{}{
							"settings": map[string]interface{}{
								"cluster_name": "test-cluster",
							},
						},
					},
				},
			},
			expected: map[string]interface{}{
				"vpc": map[string]interface{}{
					"region": "us-west-2",
				},
				"eks": map[string]interface{}{
					"cluster_name": "test-cluster",
				},
			},
			expectedFound: true,
		},
		{
			name: "no components section",
			stack: map[string]interface{}{
				"metadata": map[string]interface{}{},
			},
			expected:      nil,
			expectedFound: false,
		},
		{
			name: "no terraform section",
			stack: map[string]interface{}{
				"components": map[string]interface{}{
					"helmfile": map[string]interface{}{},
				},
			},
			expected:      nil,
			expectedFound: false,
		},
		{
			name: "no settings in components",
			stack: map[string]interface{}{
				"components": map[string]interface{}{
					"terraform": map[string]interface{}{
						"vpc": map[string]interface{}{
							"vars": map[string]interface{}{},
						},
					},
				},
			},
			expected:      nil,
			expectedFound: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, found := extractSettingsFromComponents(test.stack)
			assert.Equal(t, test.expectedFound, found)
			if found {
				assert.Equal(t, test.expected, result)
			}
		})
	}
}

func TestExtractComponentSettings(t *testing.T) {
	tests := []struct {
		name      string
		component interface{}
		expected  interface{}
	}{
		{
			name: "valid settings",
			component: map[string]interface{}{
				"settings": map[string]interface{}{
					"region": "us-west-2",
				},
			},
			expected: map[string]interface{}{
				"region": "us-west-2",
			},
		},
		{
			name: "no settings",
			component: map[string]interface{}{
				"vars": map[string]interface{}{},
			},
			expected: nil,
		},
		{
			name:      "not a map",
			component: "string value",
			expected:  nil,
		},
		{
			name: "settings not a map",
			component: map[string]interface{}{
				"settings": "string value",
			},
			expected: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := extractComponentSettings(test.component)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestDeepCopyToStringMap(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
		{
			name: "string map",
			input: map[string]interface{}{
				"key1": "value1",
				"key2": 123,
			},
			expected: map[string]interface{}{
				"key1": "value1",
				"key2": 123,
			},
		},
		{
			name: "interface map",
			input: map[interface{}]interface{}{
				"key1": "value1",
				123:    "value2",
			},
			expected: map[string]interface{}{
				"key1": "value1",
				"123":  "value2",
			},
		},
		{
			name:     "array",
			input:    []interface{}{"value1", 123},
			expected: []interface{}{"value1", 123},
		},
		{
			name: "nested maps",
			input: map[string]interface{}{
				"key1": map[interface{}]interface{}{
					"nestedKey": "nestedValue",
					123:         456,
				},
				"key2": []interface{}{
					map[interface{}]interface{}{
						"arrayKey": "arrayValue",
					},
				},
			},
			expected: map[string]interface{}{
				"key1": map[string]interface{}{
					"nestedKey": "nestedValue",
					"123":       456,
				},
				"key2": []interface{}{
					map[string]interface{}{
						"arrayKey": "arrayValue",
					},
				},
			},
		},
		{
			name:     "primitive",
			input:    "primitive value",
			expected: "primitive value",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := deepCopyToStringMap(test.input)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestHandleTerraformComponent(t *testing.T) {
	tests := []struct {
		name            string
		stack           map[string]interface{}
		component       string
		includeAbstract bool
		expected        map[string]interface{}
		expectedFound   bool
	}{
		{
			name: "regular component",
			stack: map[string]interface{}{
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
			component:       "vpc",
			includeAbstract: true,
			expected: map[string]interface{}{
				"region": "us-west-2",
			},
			expectedFound: true,
		},
		{
			name: "component with terraform prefix",
			stack: map[string]interface{}{
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
			component:       "terraform/vpc",
			includeAbstract: true,
			expected: map[string]interface{}{
				"region": "us-west-2",
			},
			expectedFound: true,
		},
		{
			name: "abstract component skip",
			stack: map[string]interface{}{
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
			component:       "vpc",
			includeAbstract: false,
			expected:        nil,
			expectedFound:   false,
		},
		{
			name: "abstract component include",
			stack: map[string]interface{}{
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
			component:       "vpc",
			includeAbstract: true,
			expected: map[string]interface{}{
				"region": "us-west-2",
			},
			expectedFound: true,
		},
		{
			name: "no components section",
			stack: map[string]interface{}{
				"metadata": map[string]interface{}{},
			},
			component:       "vpc",
			includeAbstract: true,
			expected:        nil,
			expectedFound:   false,
		},
		{
			name: "no terraform section",
			stack: map[string]interface{}{
				"components": map[string]interface{}{
					"helmfile": map[string]interface{}{},
				},
			},
			component:       "vpc",
			includeAbstract: true,
			expected:        nil,
			expectedFound:   false,
		},
		{
			name: "component not found",
			stack: map[string]interface{}{
				"components": map[string]interface{}{
					"terraform": map[string]interface{}{
						"eks": map[string]interface{}{},
					},
				},
			},
			component:       "vpc",
			includeAbstract: true,
			expected:        nil,
			expectedFound:   false,
		},
		{
			name: "no vars in component",
			stack: map[string]interface{}{
				"components": map[string]interface{}{
					"terraform": map[string]interface{}{
						"vpc": map[string]interface{}{
							"settings": map[string]interface{}{},
						},
					},
				},
			},
			component:       "vpc",
			includeAbstract: true,
			expected:        nil,
			expectedFound:   false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, found := handleTerraformComponent(test.stack, test.component, test.includeAbstract)
			assert.Equal(t, test.expectedFound, found)
			if found {
				assert.Equal(t, test.expected, result)
			}
		})
	}
}

func TestFormatArrayValue(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
		{
			name:     "string array",
			input:    []interface{}{"value1", "value2", "value3"},
			expected: "value1,value2,value3",
		},
		{
			name:     "mixed array",
			input:    []interface{}{"value1", 123, true},
			expected: "value1,123,true",
		},
		{
			name:     "empty array",
			input:    []interface{}{},
			expected: "",
		},
		{
			name:     "non-array",
			input:    "string value",
			expected: "string value",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := formatArrayValue(test.input)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestGetValueFromPath(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]interface{}
		path     string
		expected interface{}
	}{
		{
			name: "empty path returns all data",
			data: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
			path: "",
			expected: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name: "simple path",
			data: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
			path:     "key1",
			expected: "value1",
		},
		{
			name: "nested path",
			data: map[string]interface{}{
				"key1": map[string]interface{}{
					"nested": "value",
				},
			},
			path:     "key1.nested",
			expected: "value",
		},
		{
			name: "array index path",
			data: map[string]interface{}{
				"array": []interface{}{"value1", "value2"},
			},
			path:     "array.0",
			expected: "value1",
		},
		{
			name: "path with leading dot",
			data: map[string]interface{}{
				"key1": "value1",
			},
			path:     ".key1",
			expected: "value1",
		},
		{
			name: "path not found",
			data: map[string]interface{}{
				"key1": "value1",
			},
			path:     "key2",
			expected: nil,
		},
		{
			name: "nested map in array",
			data: map[string]interface{}{
				"array": []interface{}{
					map[string]interface{}{
						"key": "value",
					},
				},
			},
			path:     "array.key",
			expected: "value",
		},
		{
			name: "wildcard path",
			data: map[string]interface{}{
				"vpc": map[string]interface{}{
					"subnet1": "10.0.1.0/24",
					"subnet2": "10.0.2.0/24",
				},
			},
			path: "vpc.subnet*",
			expected: map[string]interface{}{
				"subnet1": "10.0.1.0/24",
				"subnet2": "10.0.2.0/24",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := getValueFromPath(test.data, test.path)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestNavigatePath(t *testing.T) {
	tests := []struct {
		name     string
		data     interface{}
		parts    []string
		expected interface{}
	}{
		{
			name: "navigate through map",
			data: map[string]interface{}{
				"key1": map[string]interface{}{
					"key2": "value",
				},
			},
			parts:    []string{"key1", "key2"},
			expected: "value",
		},
		{
			name: "navigate through array",
			data: map[string]interface{}{
				"array": []interface{}{"value1", "value2"},
			},
			parts:    []string{"array", "0"},
			expected: "value1",
		},
		{
			name:     "empty parts",
			data:     map[string]interface{}{"key": "value"},
			parts:    []string{""},
			expected: map[string]interface{}{"key": "value"},
		},
		{
			name: "navigate through array to map",
			data: map[string]interface{}{
				"array": []interface{}{
					map[string]interface{}{
						"key": "value",
					},
				},
			},
			parts:    []string{"array", "key"},
			expected: "value",
		},
		{
			name: "path not found",
			data: map[string]interface{}{
				"key1": "value1",
			},
			parts:    []string{"key2"},
			expected: nil,
		},
		{
			name: "handle wildcard",
			data: map[string]interface{}{
				"subnets": map[string]interface{}{
					"subnet1": "10.0.1.0/24",
					"subnet2": "10.0.2.0/24",
				},
			},
			parts: []string{"subnets", "subnet*"},
			expected: map[string]interface{}{
				"subnet1": "10.0.1.0/24",
				"subnet2": "10.0.2.0/24",
			},
		},
		{
			name:     "non-map and non-array data",
			data:     "string value",
			parts:    []string{"key"},
			expected: nil,
		},
		{
			name: "array with non-map elements",
			data: []interface{}{
				"value1", "value2",
			},
			parts:    []string{"name"},
			expected: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := navigatePath(test.data, test.parts)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestProcessMapPart(t *testing.T) {
	tests := []struct {
		name         string
		mapData      map[string]interface{}
		part         string
		expected     interface{}
		expectedBool bool
	}{
		{
			name: "direct key match",
			mapData: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
			part:         "key1",
			expected:     "value1",
			expectedBool: true,
		},
		{
			name: "wildcard match single key",
			mapData: map[string]interface{}{
				"subnet1": "10.0.1.0/24",
				"network": "10.0.0.0/16",
			},
			part:         "subnet*",
			expected:     "10.0.1.0/24",
			expectedBool: true,
		},
		{
			name: "wildcard match multiple keys",
			mapData: map[string]interface{}{
				"subnet1": "10.0.1.0/24",
				"subnet2": "10.0.2.0/24",
				"network": "10.0.0.0/16",
			},
			part: "subnet*",
			expected: map[string]interface{}{
				"subnet1": "10.0.1.0/24",
				"subnet2": "10.0.2.0/24",
			},
			expectedBool: true,
		},
		{
			name: "key not found",
			mapData: map[string]interface{}{
				"key1": "value1",
			},
			part:         "key2",
			expected:     nil,
			expectedBool: false,
		},
		{
			name: "wildcard no match",
			mapData: map[string]interface{}{
				"net1": "10.0.1.0/24",
				"net2": "10.0.2.0/24",
			},
			part:         "subnet*",
			expected:     nil,
			expectedBool: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, found := processMapPart(test.mapData, test.part)
			assert.Equal(t, test.expectedBool, found)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestProcessWildcardPattern(t *testing.T) {
	tests := []struct {
		name         string
		mapData      map[string]interface{}
		pattern      string
		expected     interface{}
		expectedBool bool
	}{
		{
			name: "single wildcard match",
			mapData: map[string]interface{}{
				"test1": "value1",
				"other": "value2",
			},
			pattern:      "test*",
			expected:     "value1",
			expectedBool: true,
		},
		{
			name: "multiple wildcard matches",
			mapData: map[string]interface{}{
				"test1": "value1",
				"test2": "value2",
				"other": "value3",
			},
			pattern: "test*",
			expected: map[string]interface{}{
				"test1": "value1",
				"test2": "value2",
			},
			expectedBool: true,
		},
		{
			name: "no matches",
			mapData: map[string]interface{}{
				"prod1": "value1",
				"prod2": "value2",
			},
			pattern:      "test*",
			expected:     nil,
			expectedBool: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, found := processWildcardPattern(test.mapData, test.pattern)
			assert.Equal(t, test.expectedBool, found)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestProcessArrayPart(t *testing.T) {
	tests := []struct {
		name         string
		arrayData    []interface{}
		part         string
		expected     interface{}
		expectedBool bool
	}{
		{
			name:         "index access",
			arrayData:    []interface{}{"value1", "value2", "value3"},
			part:         "1",
			expected:     "value2",
			expectedBool: true,
		},
		{
			name: "map key access",
			arrayData: []interface{}{
				map[string]interface{}{
					"name": "item1",
				},
			},
			part:         "name",
			expected:     "item1",
			expectedBool: true,
		},
		{
			name:         "invalid index",
			arrayData:    []interface{}{"value1", "value2"},
			part:         "5",
			expected:     nil,
			expectedBool: false,
		},
		{
			name:         "non-numeric part",
			arrayData:    []interface{}{"value1", "value2"},
			part:         "key",
			expected:     nil,
			expectedBool: false,
		},
		{
			name:         "empty array",
			arrayData:    []interface{}{},
			part:         "0",
			expected:     nil,
			expectedBool: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, found := processArrayPart(test.arrayData, test.part)
			assert.Equal(t, test.expectedBool, found)
			if found {
				assert.Equal(t, test.expected, result)
			}
		})
	}
}
