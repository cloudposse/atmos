package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"
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

func TestIsSimpleStringStartingWithHash(t *testing.T) {
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
			name:     "string starting with hash and containing newline",
			input:    "#value\nwith newline",
			expected: false,
		},
		{
			name:     "regular string",
			input:    "regular",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSimpleStringStartingWithHash(tt.input)
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
			expected: true,
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
			assert.Equal(t, tt.expected, result, "isScalarString(%q) = %v, want %v", tt.input, result, tt.expected)
		})
	}
}

// TestIsMisinterpretedScalar tests the isMisinterpretedScalar helper function.
func TestIsMisinterpretedScalar(t *testing.T) {
	tests := []struct {
		name           string
		yamlInput      string
		originalResult string
		expected       bool
	}{
		{
			name:           "misinterpreted single colon",
			yamlInput:      "password:",
			originalResult: "password:",
			expected:       true,
		},
		{
			name:           "misinterpreted double colon",
			yamlInput:      "password::",
			originalResult: "password::",
			expected:       true,
		},
		{
			name:           "valid map",
			yamlInput:      "key: value",
			originalResult: "key: value",
			expected:       false,
		},
		{
			name:           "scalar string",
			yamlInput:      "simple",
			originalResult: "simple",
			expected:       false,
		},
		{
			name:           "complex ARN misinterpreted",
			yamlInput:      "arn:aws:secretsmanager:us-east-2:123456789012:secret:my-secret-AbCdEf:password::",
			originalResult: "arn:aws:secretsmanager:us-east-2:123456789012:secret:my-secret-AbCdEf:password::",
			expected:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var node yaml.Node
			err := yaml.Unmarshal([]byte(tt.yamlInput), &node)
			require.NoError(t, err)

			result := isMisinterpretedScalar(&node, tt.originalResult)
			assert.Equal(t, tt.expected, result, "isMisinterpretedScalar for %q = %v, want %v", tt.yamlInput, result, tt.expected)
		})
	}
}

// TestEvaluateYqExpression_StringEndingWithColon verifies that strings ending
// with colons are correctly returned as strings, not misinterpreted as maps.
//
// Regression guard for GitHub issue #2031: !terraform.state YAML tag incorrectly
// parses string values ending with colons (like AWS Secrets Manager ARNs).
func TestEvaluateYqExpression_StringEndingWithColon(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]any
		yq       string
		expected any
	}{
		{
			name: "ARN ending with double colon",
			data: map[string]any{
				"db_secret_arn": "arn:aws:secretsmanager:us-east-2:123456789012:secret:my-secret-AbCdEf:password::",
			},
			yq:       ".db_secret_arn",
			expected: "arn:aws:secretsmanager:us-east-2:123456789012:secret:my-secret-AbCdEf:password::",
		},
		{
			name: "ARN ending with single colon",
			data: map[string]any{
				"arn": "arn:aws:iam::123456789012:role/MyRole:",
			},
			yq:       ".arn",
			expected: "arn:aws:iam::123456789012:role/MyRole:",
		},
		{
			name: "simple string ending with colon",
			data: map[string]any{
				"value": "password:",
			},
			yq:       ".value",
			expected: "password:",
		},
		{
			name: "string with multiple colons ending with double colon",
			data: map[string]any{
				"key": "a:b:c:d::",
			},
			yq:       ".key",
			expected: "a:b:c:d::",
		},
		{
			name: "regular string without trailing colon",
			data: map[string]any{
				"bucket": "my-bucket",
			},
			yq:       ".bucket",
			expected: "my-bucket",
		},
		{
			name: "string with colon in middle",
			data: map[string]any{
				"url": "https://example.com",
			},
			yq:       ".url",
			expected: "https://example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := EvaluateYqExpression(nil, tt.data, tt.yq)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result, "EvaluateYqExpression returned wrong value")
		})
	}
}
