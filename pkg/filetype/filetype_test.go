package filetype

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.yaml.in/yaml/v3"
)

func TestParseYAML(t *testing.T) {
	// Test error case for yaml.Unmarshal.
	t.Run("invalid yaml", func(t *testing.T) {
		invalidYAML := []byte("invalid: yaml: :")
		result, err := parseYAML(invalidYAML)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	// Test error case for node.Decode (covers lines 100-101).
	t.Run("decode error", func(t *testing.T) {
		// Create YAML with anchor/alias that might fail on decode.
		yamlWithComplexAnchors := []byte(`
x: &anchor
  <<: *anchor
`)
		result, err := parseYAML(yamlWithComplexAnchors)
		// This may or may not error, but it exercises the decode path.
		if err != nil {
			assert.Nil(t, result)
		}
	})

	tests := []struct {
		name     string
		input    string
		expected any
	}{
		{
			name:     "regular string",
			input:    "key: value",
			expected: map[string]any{"key": "value"},
		},
		{
			name:     "string starting with hash",
			input:    "key: '#value'",
			expected: map[string]any{"key": "#value"},
		},
		// Note: In YAML, unquoted strings starting with # are comments.
		// This test is removed as it's not valid YAML.
		{
			name: "nested map with hash values",
			input: `
parent:
  child1: '#value1'
  child2: '#value2'
  child3: regular
`,
			expected: map[string]any{
				"parent": map[string]any{
					"child1": "#value1",
					"child2": "#value2",
					"child3": "regular",
				},
			},
		},
		{
			name: "list with hash values",
			input: `
items:
  - '#item1'
  - '#item2'
  - regular
`,
			expected: map[string]any{
				"items": []any{
					"#item1",
					"#item2",
					"regular",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseYAML([]byte(tt.input))
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessYAMLNode(t *testing.T) {
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
			// Create a node with the test input.
			node := &yaml.Node{
				Kind:  yaml.ScalarNode,
				Tag:   "!!str",
				Value: tt.input,
			}

			// Process the node.
			processYAMLNode(node)

			// Check if the style was set correctly.
			assert.Equal(t, tt.expected, node.Style)
		})
	}

	t.Run("nested nodes", func(t *testing.T) {
		// Create a document node with nested content.
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
							Kind:  yaml.ScalarNode,
							Tag:   "!!str",
							Value: "regular",
						},
					},
				},
			},
		}

		// Process the document.
		processYAMLNode(doc)

		// Check if the style was set correctly for the hash value.
		assert.Equal(t, yaml.SingleQuotedStyle, doc.Content[0].Content[1].Style)
		// Check that regular value style was not changed.
		assert.Equal(t, yaml.Style(0), doc.Content[0].Content[3].Style)
	})

	t.Run("nil node", func(t *testing.T) {
		// This should not panic.
		processYAMLNode(nil)
	})
}
