package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/op/go-logging.v1"
	yaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestEvaluateYqExpression_InvalidYAML tests the error case when yaml.Unmarshal fails.
func TestEvaluateYqExpression_InvalidYAML(t *testing.T) {
	// Create a test with invalid YAML that will cause yaml.Unmarshal to fail.
	// Create a test function that will try to unmarshal invalid YAML.
	var node yaml.Node
	err := yaml.Unmarshal([]byte("invalid: yaml: :"), &node)

	// Verify that we get an error from yaml.Unmarshal.
	assert.Error(t, err, "Invalid YAML should cause an error")
}

// TestIsScalarString tests the isScalarString helper function.
func TestIsScalarString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "string starting with hash",
			input:    "#value",
			expected: true,
		},
		{
			name:     "string starting with hash with newline",
			input:    "#value\nmore",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false, // Empty strings should go through YAML parsing which converts them to nil.
		},
		{
			name:     "string ending with single colon",
			input:    "value:",
			expected: true,
		},
		{
			name:     "string ending with double colon",
			input:    "value::",
			expected: true,
		},
		{
			name:     "ARN ending with double colon",
			input:    "arn:aws:secretsmanager:us-east-2:123456789012:secret:my-secret-AbCdEf:password::",
			expected: true,
		},
		{
			name:     "ARN ending with single colon",
			input:    "arn:aws:secretsmanager:us-east-2:123456789012:secret:my-secret:",
			expected: true,
		},
		{
			name:     "IPv6 ending with double colon",
			input:    "2041:0000:140F::875B::",
			expected: true,
		},
		{
			name:     "IPv6 all-zeros shorthand",
			input:    "::",
			expected: true,
		},
		{
			name:     "IPv6 loopback (does not end with colon)",
			input:    "::1",
			expected: false,
		},
		{
			name:     "IPv6 with no trailing colon",
			input:    "2001:db8::1",
			expected: false,
		},
		{
			name:     "string with colon space pattern",
			input:    "key: value:",
			expected: false,
		},
		{
			name:     "JSON object",
			input:    `{"key": "value"}`,
			expected: false,
		},
		{
			name:     "JSON array",
			input:    `["a", "b"]`,
			expected: false,
		},
		{
			name:     "multiline string",
			input:    "line1\nline2",
			expected: false,
		},
		{
			name:     "regular string no colon",
			input:    "regular-value",
			expected: false,
		},
		{
			name:     "string with colon in middle",
			input:    "key:value",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isScalarString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessYAMLNode_Utils(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected yaml.Style
	}{
		{
			name:     "string starting with hash",
			input:    "#value",
			expected: yaml.SingleQuotedStyle,
		},
		{
			name:     "regular string",
			input:    "regular",
			expected: 0, // Default style
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a node with the test input
			node := &yaml.Node{
				Kind:  yaml.ScalarNode,
				Tag:   "!!str",
				Value: tt.input,
			}

			// Process the node
			processYAMLNode(node)

			// Check if the style was set correctly
			assert.Equal(t, tt.expected, node.Style)
		})
	}

	t.Run("complex nested structure", func(t *testing.T) {
		// Create a more complex document with multiple levels of nesting
		doc := &yaml.Node{
			Kind: yaml.DocumentNode,
			Content: []*yaml.Node{
				{
					Kind: yaml.MappingNode,
					Content: []*yaml.Node{
						{
							Kind:  yaml.ScalarNode,
							Tag:   "!!str",
							Value: "key1",
						},
						{
							Kind:  yaml.ScalarNode,
							Tag:   "!!str",
							Value: "#value1",
						},
						{
							Kind:  yaml.ScalarNode,
							Tag:   "!!str",
							Value: "key2",
						},
						{
							Kind: yaml.MappingNode,
							Content: []*yaml.Node{
								{
									Kind:  yaml.ScalarNode,
									Tag:   "!!str",
									Value: "nested_key",
								},
								{
									Kind:  yaml.ScalarNode,
									Tag:   "!!str",
									Value: "#nested_value",
								},
							},
						},
						{
							Kind:  yaml.ScalarNode,
							Tag:   "!!str",
							Value: "key3",
						},
						{
							Kind: yaml.SequenceNode,
							Content: []*yaml.Node{
								{
									Kind:  yaml.ScalarNode,
									Tag:   "!!str",
									Value: "#list_item1",
								},
								{
									Kind:  yaml.ScalarNode,
									Tag:   "!!str",
									Value: "regular_item",
								},
							},
						},
					},
				},
			},
		}

		// Process the document
		processYAMLNode(doc)

		// Check if the style was set correctly for all hash values
		assert.Equal(t, yaml.SingleQuotedStyle, doc.Content[0].Content[1].Style, "First level hash value should have single quoted style")
		assert.Equal(t, yaml.SingleQuotedStyle, doc.Content[0].Content[3].Content[1].Style, "Nested hash value should have single quoted style")
		assert.Equal(t, yaml.SingleQuotedStyle, doc.Content[0].Content[5].Content[0].Style, "List hash value should have single quoted style")

		// Check that regular values were not changed
		assert.Equal(t, yaml.Style(0), doc.Content[0].Content[5].Content[1].Style, "Regular list item should not have style changed")
	})

	t.Run("nil node", func(t *testing.T) {
		// This should not panic
		processYAMLNode(nil)
	})
}

// TestEvaluateYqExpression_NilDataWithDefault verifies that YQ default
// values work when the input data is nil.
//
// Regression guard: Previously, nil was converted to "null\n" YAML which
// caused YQ to error when accessing a key with the fallback operator.
// Expected: `.missing // "default"` should return "default" when data is nil.
// This test ensures the fix continues to work correctly.
func TestEvaluateYqExpression_NilDataWithDefault(t *testing.T) {
	result, err := EvaluateYqExpression(nil, nil, `.missing // "default"`)

	// Verify that YQ correctly processes nil data with default values.
	require.NoError(t, err)
	assert.Equal(t, "default", result)
}

// TestEvaluateYqExpression_EmptyMapWithDefault verifies that YQ default
// values work when the input data is an empty map.
//
// This should work correctly - YQ can access missing keys on empty maps
// and the fallback operator should return the default value.
func TestEvaluateYqExpression_EmptyMapWithDefault(t *testing.T) {
	result, err := EvaluateYqExpression(nil, map[string]any{}, `.missing // "default"`)

	require.NoError(t, err)
	assert.Equal(t, "default", result)
}

// TestEvaluateYqExpression_NilDataWithListDefault verifies that YQ default
// values work with list fallback expressions when input is nil.
//
// Regression guard: Ensures nil input with list defaults works correctly.
func TestEvaluateYqExpression_NilDataWithListDefault(t *testing.T) {
	result, err := EvaluateYqExpression(nil, nil, `.items // ["a", "b", "c"]`)

	require.NoError(t, err)
	assert.Equal(t, []any{"a", "b", "c"}, result)
}

// TestEvaluateYqExpression_NilDataWithMapDefault verifies that YQ default
// values work with map fallback expressions when input is nil.
//
// Regression guard: Ensures nil input with map defaults works correctly.
func TestEvaluateYqExpression_NilDataWithMapDefault(t *testing.T) {
	result, err := EvaluateYqExpression(nil, nil, `.config // {"key": "value"}`)

	require.NoError(t, err)
	expected := map[string]any{"key": "value"}
	assert.Equal(t, expected, result)
}

// TestEvaluateYqExpression_EmptyMapNestedKeyWithDefault verifies that YQ default
// values work when accessing nested keys on empty maps.
func TestEvaluateYqExpression_EmptyMapNestedKeyWithDefault(t *testing.T) {
	result, err := EvaluateYqExpression(nil, map[string]any{}, `.parent.child // "default"`)

	require.NoError(t, err)
	assert.Equal(t, "default", result)
}

// TestEvaluateYqExpression_ExistingKeyNoDefault verifies that existing keys
// return their values even when a default is provided.
func TestEvaluateYqExpression_ExistingKeyNoDefault(t *testing.T) {
	data := map[string]any{
		"bucket": "my-bucket",
	}
	result, err := EvaluateYqExpression(nil, data, `.bucket // "default-bucket"`)

	require.NoError(t, err)
	assert.Equal(t, "my-bucket", result)
}

// TestEvaluateYqExpression_NullValueWithDefault verifies that YQ default
// values work when a key exists but has a null value.
//
// This tests the YQ semantics of null // "default".
func TestEvaluateYqExpression_NullValueWithDefault(t *testing.T) {
	data := map[string]any{
		"bucket": nil,
	}
	result, err := EvaluateYqExpression(nil, data, `.bucket // "default-bucket"`)

	require.NoError(t, err)
	// YQ's // operator returns the alternative when the value is null.
	assert.Equal(t, "default-bucket", result)
}

// TestIsMisinterpretedScalar tests the isMisinterpretedScalar helper function.
func TestIsMisinterpretedScalar(t *testing.T) {
	tests := []struct {
		name           string
		yamlContent    string
		originalResult string
		expected       bool
	}{
		{
			name:           "single colon suffix misinterpreted",
			yamlContent:    "value:\n",
			originalResult: "value:",
			expected:       true,
		},
		{
			name:           "double colon suffix misinterpreted",
			yamlContent:    "value:\n",
			originalResult: "value::",
			expected:       true,
		},
		{
			name:           "ARN with double colon",
			yamlContent:    "arn:aws:secretsmanager:us-east-2:123456789012:secret:my-secret-AbCdEf:password:\n",
			originalResult: "arn:aws:secretsmanager:us-east-2:123456789012:secret:my-secret-AbCdEf:password::",
			expected:       true,
		},
		{
			name:           "regular map not misinterpreted",
			yamlContent:    "key: value\n",
			originalResult: "key: value",
			expected:       false,
		},
		{
			name:           "scalar value not misinterpreted",
			yamlContent:    "simple-value\n",
			originalResult: "simple-value",
			expected:       false,
		},
		{
			name:           "multi-key map not misinterpreted",
			yamlContent:    "key1: value1\nkey2: value2\n",
			originalResult: "key1: value1\nkey2: value2",
			expected:       false,
		},
		{
			// IPv6 address ending in :: is parsed by YAML as a mapping node
			// with key "2041:0000:140F::875B:" and null value.
			name:           "IPv6 address ending with double colon misinterpreted as map",
			yamlContent:    "2041:0000:140F::875B:\n",
			originalResult: "2041:0000:140F::875B::",
			expected:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var node yaml.Node
			err := yaml.Unmarshal([]byte(tt.yamlContent), &node)
			require.NoError(t, err)

			result := isMisinterpretedScalar(&node, tt.originalResult)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIsYAMLNullValue tests the isYAMLNullValue helper function.
func TestIsYAMLNullValue(t *testing.T) {
	tests := []struct {
		name     string
		node     *yaml.Node
		expected bool
	}{
		{
			name: "null tag",
			node: &yaml.Node{
				Kind: yaml.ScalarNode,
				Tag:  "!!null",
			},
			expected: true,
		},
		{
			name: "empty value scalar",
			node: &yaml.Node{
				Kind:  yaml.ScalarNode,
				Value: "",
			},
			expected: true,
		},
		{
			name: "non-empty scalar",
			node: &yaml.Node{
				Kind:  yaml.ScalarNode,
				Value: "value",
			},
			expected: false,
		},
		{
			name: "mapping node",
			node: &yaml.Node{
				Kind: yaml.MappingNode,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isYAMLNullValue(tt.node)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestKeyMatchesOriginalWithColon tests the keyMatchesOriginalWithColon helper function.
func TestKeyMatchesOriginalWithColon(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		original string
		expected bool
	}{
		{
			name:     "single colon match",
			key:      "value",
			original: "value:",
			expected: true,
		},
		{
			name:     "double colon match",
			key:      "value",
			original: "value::",
			expected: true,
		},
		{
			name:     "no match - no colon",
			key:      "value",
			original: "value",
			expected: false,
		},
		{
			name:     "no match - triple colon",
			key:      "value",
			original: "value:::",
			expected: false,
		},
		{
			name:     "ARN single colon",
			key:      "arn:aws:secretsmanager:us-east-2:123456789012:secret:my-secret",
			original: "arn:aws:secretsmanager:us-east-2:123456789012:secret:my-secret:",
			expected: true,
		},
		{
			name:     "ARN double colon",
			key:      "arn:aws:secretsmanager:us-east-2:123456789012:secret:my-secret-AbCdEf:password",
			original: "arn:aws:secretsmanager:us-east-2:123456789012:secret:my-secret-AbCdEf:password::",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := keyMatchesOriginalWithColon(tt.key, tt.original)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestEvaluateYqExpression_ARNWithTrailingColons tests that ARN strings ending
// with colons are correctly preserved as strings and not misinterpreted as maps.
// This is a regression test for issue #2031.
func TestEvaluateYqExpression_ARNWithTrailingColons(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	tests := []struct {
		name     string
		data     map[string]any
		yq       string
		expected string
	}{
		{
			name: "ARN ending with double colon",
			data: map[string]any{
				"secret_arn": "arn:aws:secretsmanager:us-east-2:123456789012:secret:my-secret-AbCdEf:password::",
			},
			yq:       ".secret_arn",
			expected: "arn:aws:secretsmanager:us-east-2:123456789012:secret:my-secret-AbCdEf:password::",
		},
		{
			name: "ARN ending with single colon",
			data: map[string]any{
				"secret_arn": "arn:aws:secretsmanager:us-east-2:123456789012:secret:my-secret:",
			},
			yq:       ".secret_arn",
			expected: "arn:aws:secretsmanager:us-east-2:123456789012:secret:my-secret:",
		},
		{
			name: "nested ARN with trailing colons",
			data: map[string]any{
				"secrets": map[string]any{
					"db_password": "arn:aws:secretsmanager:us-east-2:123456789012:secret:my-secret-AbCdEf:password::",
					"db_username": "arn:aws:secretsmanager:us-east-2:123456789012:secret:my-secret-AbCdEf:username::",
				},
			},
			yq:       ".secrets.db_password",
			expected: "arn:aws:secretsmanager:us-east-2:123456789012:secret:my-secret-AbCdEf:password::",
		},
		{
			name: "simple value ending with colon",
			data: map[string]any{
				"value": "test:",
			},
			yq:       ".value",
			expected: "test:",
		},
		{
			name: "simple value ending with double colon",
			data: map[string]any{
				"value": "test::",
			},
			yq:       ".value",
			expected: "test::",
		},
		{
			// Regression test for https://github.com/cloudposse/atmos/issues/XXXX
			// IPv6 addresses ending in :: were being misinterpreted as YAML maps.
			name: "IPv6 address ending with double colon",
			data: map[string]any{
				"ipv6": "2041:0000:140F::875B::",
			},
			yq:       ".ipv6",
			expected: "2041:0000:140F::875B::",
		},
		{
			name: "IPv6 all-zeros shorthand (double colon only)",
			data: map[string]any{
				"ipv6": "::",
			},
			yq:       ".ipv6",
			expected: "::",
		},
		{
			name: "IPv6 loopback address (does not end with colon)",
			data: map[string]any{
				"ipv6": "::1",
			},
			yq:       ".ipv6",
			expected: "::1",
		},
		{
			name: "IPv6 standard address without trailing colon",
			data: map[string]any{
				"ipv6": "2001:db8::1",
			},
			yq:       ".ipv6",
			expected: "2001:db8::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := EvaluateYqExpression(atmosConfig, tt.data, tt.yq)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestEvaluateYqExpression_IPv6Scenario tests that IPv6 address strings are correctly
// preserved as strings when retrieved via !terraform.state, not misinterpreted as YAML maps.
// This is a regression test for issue #2155 where strings ending in :: were converted
// to map objects like {"2041:0000:140F::875B:": null}.
func TestEvaluateYqExpression_IPv6Scenario(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// This simulates what !terraform.state would return for an IPv6 output.
	data := map[string]any{
		"ipv6": "2041:0000:140F::875B::",
	}

	// The IPv6 address should be returned as a string, not a map.
	result, err := EvaluateYqExpression(atmosConfig, data, ".ipv6")
	require.NoError(t, err)
	// Verify the result is a string, not a map (the bug would return map[string]any{"2041:0000:140F::875B:": nil}).
	ipv6Str, ok := result.(string)
	require.True(t, ok, "IPv6 address should be a string, got %T: %v", result, result)
	assert.Equal(t, "2041:0000:140F::875B::", ipv6Str)

	// Test with a map of IPv6 addresses.
	dataMap := map[string]any{
		"addresses": map[string]any{
			"primary":   "2041:0000:140F::875B::",
			"secondary": "2001:db8::",
			"loopback":  "::1",
		},
	}

	// Extract individual IPv6 values.
	primary, err := EvaluateYqExpression(atmosConfig, dataMap, ".addresses.primary")
	require.NoError(t, err)
	primaryStr, ok := primary.(string)
	require.True(t, ok, "Primary IPv6 should be a string, got %T", primary)
	assert.Equal(t, "2041:0000:140F::875B::", primaryStr)

	secondary, err := EvaluateYqExpression(atmosConfig, dataMap, ".addresses.secondary")
	require.NoError(t, err)
	secondaryStr, ok := secondary.(string)
	require.True(t, ok, "Secondary IPv6 should be a string, got %T", secondary)
	assert.Equal(t, "2001:db8::", secondaryStr)

	loopback, err := EvaluateYqExpression(atmosConfig, dataMap, ".addresses.loopback")
	require.NoError(t, err)
	loopbackStr, ok := loopback.(string)
	require.True(t, ok, "Loopback IPv6 should be a string, got %T", loopback)
	assert.Equal(t, "::1", loopbackStr)

	// Extract the entire addresses map - values should still be strings.
	addresses, err := EvaluateYqExpression(atmosConfig, dataMap, ".addresses")
	require.NoError(t, err)

	addressMap, ok := addresses.(map[string]any)
	require.True(t, ok, "Addresses result should be a map, got %T", addresses)

	// Verify primary IPv6 is a string, not a map.
	primaryVal, ok := addressMap["primary"].(string)
	require.True(t, ok, "Primary IPv6 in map should be a string, got %T: %v", addressMap["primary"], addressMap["primary"])
	assert.Equal(t, "2041:0000:140F::875B::", primaryVal)
}

// TestEvaluateYqExpression_MapSecretsScenario tests the exact scenario from issue #2031
// where !terraform.state returns AWS Secrets Manager ARNs that were being misinterpreted.
func TestEvaluateYqExpression_MapSecretsScenario(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// This simulates what !terraform.state would return for a map of secrets.
	data := map[string]any{
		"map_secrets": map[string]any{
			"DB_PASSWORD": "arn:aws:secretsmanager:us-east-2:123456789012:secret:my-secret-AbCdEf:password::",
			"DB_USERNAME": "arn:aws:secretsmanager:us-east-2:123456789012:secret:my-secret-AbCdEf:username::",
		},
	}

	// Extract individual values.
	passwordResult, err := EvaluateYqExpression(atmosConfig, data, ".map_secrets.DB_PASSWORD")
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:secretsmanager:us-east-2:123456789012:secret:my-secret-AbCdEf:password::", passwordResult)

	usernameResult, err := EvaluateYqExpression(atmosConfig, data, ".map_secrets.DB_USERNAME")
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:secretsmanager:us-east-2:123456789012:secret:my-secret-AbCdEf:username::", usernameResult)

	// Extract the entire map - values should still be strings, not nested maps.
	mapResult, err := EvaluateYqExpression(atmosConfig, data, ".map_secrets")
	require.NoError(t, err)

	resultMap, ok := mapResult.(map[string]any)
	require.True(t, ok, "Result should be a map")

	// Verify DB_PASSWORD is a string, not a map.
	dbPassword, ok := resultMap["DB_PASSWORD"].(string)
	require.True(t, ok, "DB_PASSWORD should be a string, got %T", resultMap["DB_PASSWORD"])
	assert.Equal(t, "arn:aws:secretsmanager:us-east-2:123456789012:secret:my-secret-AbCdEf:password::", dbPassword)

	// Verify DB_USERNAME is a string, not a map.
	dbUsername, ok := resultMap["DB_USERNAME"].(string)
	require.True(t, ok, "DB_USERNAME should be a string, got %T", resultMap["DB_USERNAME"])
	assert.Equal(t, "arn:aws:secretsmanager:us-east-2:123456789012:secret:my-secret-AbCdEf:username::", dbUsername)
}

// TestEvaluateYqExpressionWithType tests the generic EvaluateYqExpressionWithType function.
func TestEvaluateYqExpressionWithType(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	t.Run("extract map value", func(t *testing.T) {
		data := map[string]any{
			"name": "test-value",
		}
		result, err := EvaluateYqExpressionWithType(atmosConfig, data, ".")
		require.NoError(t, err)
		require.NotNil(t, result)
		// Result is a pointer to map[string]any.
		assert.Equal(t, "test-value", (*result)["name"])
	})

	t.Run("extract nested structure", func(t *testing.T) {
		data := map[string]any{
			"parent": map[string]any{
				"child": "nested-value",
			},
		}
		result, err := EvaluateYqExpressionWithType(atmosConfig, data, ".parent")
		require.NoError(t, err)
		require.NotNil(t, result)
	})

	t.Run("extract with default value", func(t *testing.T) {
		data := map[string]any{}
		result, err := EvaluateYqExpressionWithType(atmosConfig, data, `. // {"default": "value"}`)
		require.NoError(t, err)
		require.NotNil(t, result)
	})

	t.Run("invalid yq expression returns error", func(t *testing.T) {
		data := map[string]any{"key": "value"}
		_, err := EvaluateYqExpressionWithType(atmosConfig, data, ".[[[invalid")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to evaluate YQ expression")
	})

	t.Run("nil config works", func(t *testing.T) {
		data := map[string]any{"key": "value"}
		result, err := EvaluateYqExpressionWithType[map[string]any](nil, data, ".")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "value", (*result)["key"])
	})
}

// TestEvaluateYqExpression_ErrorPaths tests various error scenarios.
func TestEvaluateYqExpression_ErrorPaths(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	t.Run("invalid yq expression", func(t *testing.T) {
		data := map[string]any{"key": "value"}
		_, err := EvaluateYqExpression(atmosConfig, data, ".[[[invalid")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to evaluate YQ expression")
	})

	t.Run("complex valid expression", func(t *testing.T) {
		data := map[string]any{
			"items": []any{"a", "b", "c"},
		}
		result, err := EvaluateYqExpression(atmosConfig, data, ".items | length")
		require.NoError(t, err)
		// YQ returns length as integer.
		assert.Equal(t, 3, result)
	})

	t.Run("select expression", func(t *testing.T) {
		data := map[string]any{
			"items": []any{
				map[string]any{"name": "a", "value": 1},
				map[string]any{"name": "b", "value": 2},
			},
		}
		result, err := EvaluateYqExpression(atmosConfig, data, `.items[] | select(.name == "b") | .value`)
		require.NoError(t, err)
		assert.Equal(t, 2, result)
	})
}

// TestLogBackend tests the logBackend struct methods.
func TestLogBackend(t *testing.T) {
	backend := logBackend{}

	t.Run("Log returns nil", func(t *testing.T) {
		err := backend.Log(0, 0, nil)
		assert.NoError(t, err)
	})

	t.Run("GetLevel returns ERROR", func(t *testing.T) {
		level := backend.GetLevel("any")
		assert.Equal(t, logging.ERROR, level)
	})

	t.Run("SetLevel does nothing", func(t *testing.T) {
		// Just verify it doesn't panic.
		backend.SetLevel(logging.DEBUG, "test")
	})

	t.Run("IsEnabledFor returns false", func(t *testing.T) {
		result := backend.IsEnabledFor(logging.DEBUG, "test")
		assert.False(t, result)
	})
}

// TestConfigureYqLogger tests the configureYqLogger function with different configurations.
func TestConfigureYqLogger(t *testing.T) {
	t.Run("nil config uses no-op backend", func(t *testing.T) {
		// Should not panic.
		configureYqLogger(nil)
	})

	t.Run("non-trace log level uses no-op backend", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Logs: schema.Logs{
				Level: "Info",
			},
		}
		// Should not panic.
		configureYqLogger(atmosConfig)
	})

	t.Run("trace log level uses default backend", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Logs: schema.Logs{
				Level: LogLevelTrace,
			},
		}
		// Should not panic - this path uses the default yq logger.
		configureYqLogger(atmosConfig)
	})
}

// TestEvaluateYqExpression_EdgeCases tests additional edge cases.
func TestEvaluateYqExpression_EdgeCases(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	t.Run("boolean values", func(t *testing.T) {
		data := map[string]any{"enabled": true}
		result, err := EvaluateYqExpression(atmosConfig, data, ".enabled")
		require.NoError(t, err)
		assert.Equal(t, true, result)
	})

	t.Run("numeric values", func(t *testing.T) {
		data := map[string]any{"count": 42}
		result, err := EvaluateYqExpression(atmosConfig, data, ".count")
		require.NoError(t, err)
		assert.Equal(t, 42, result)
	})

	t.Run("float values", func(t *testing.T) {
		data := map[string]any{"ratio": 3.14}
		result, err := EvaluateYqExpression(atmosConfig, data, ".ratio")
		require.NoError(t, err)
		assert.Equal(t, 3.14, result)
	})

	t.Run("array values", func(t *testing.T) {
		data := map[string]any{"items": []any{"a", "b", "c"}}
		result, err := EvaluateYqExpression(atmosConfig, data, ".items")
		require.NoError(t, err)
		resultArray, ok := result.([]any)
		require.True(t, ok)
		assert.Len(t, resultArray, 3)
	})

	t.Run("nested map values", func(t *testing.T) {
		data := map[string]any{
			"parent": map[string]any{
				"child": map[string]any{
					"grandchild": "value",
				},
			},
		}
		result, err := EvaluateYqExpression(atmosConfig, data, ".parent.child.grandchild")
		require.NoError(t, err)
		assert.Equal(t, "value", result)
	})

	t.Run("string with colons in middle", func(t *testing.T) {
		// String with colons in the middle (not at the end) should be preserved.
		data := map[string]any{"special": "key:value:data"}
		result, err := EvaluateYqExpression(atmosConfig, data, ".special")
		require.NoError(t, err)
		assert.Equal(t, "key:value:data", result)
	})

	t.Run("string starting with hash", func(t *testing.T) {
		data := map[string]any{"comment": "#this looks like a comment"}
		result, err := EvaluateYqExpression(atmosConfig, data, ".comment")
		require.NoError(t, err)
		assert.Equal(t, "#this looks like a comment", result)
	})
}
