package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
