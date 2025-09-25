package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

// TestYamlNodeProcessingPreservesNewlines tests that YAML node processing preserves newlines.
func TestYamlNodeProcessingPreservesNewlines(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "value with trailing newline",
			input:    "hello world\n",
			expected: "hello world\n",
		},
		{
			name:     "multiline value with newlines",
			input:    "line1\nline2\nline3\n",
			expected: "line1\nline2\nline3\n",
		},
		{
			name:     "value with leading newline",
			input:    "\nhello",
			expected: "\nhello",
		},
		{
			name:     "value with multiple consecutive newlines",
			input:    "hello\n\n\nworld",
			expected: "hello\n\n\nworld",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a YAML node with a custom tag.
			node := &yaml.Node{
				Tag:   "!terraform.output",
				Value: tc.input,
				Kind:  yaml.ScalarNode,
			}

			// Test getValueWithTag function.
			result := getValueWithTag(node)

			// The current implementation strips newlines, which is the bug.
			// We expect this test to fail initially, demonstrating the issue.
			expectedWithTag := "!terraform.output " + tc.expected

			// This assertion will likely fail due to the TrimSpace calls.
			assert.Equal(t, expectedWithTag, result,
				"getValueWithTag should preserve newlines in the value")
		})
	}
}

// TestProcessCustomTagsPreservesWhitespace tests the processCustomTags function.
func TestProcessCustomTagsPreservesWhitespace(t *testing.T) {
	yamlContent := `
components:
  terraform:
    test:
      vars:
        multiline: !terraform.output "component stack output_with_newlines"
        trailing: !terraform.output "component stack output_with_trailing\n"
`

	var node yaml.Node
	err := yaml.Unmarshal([]byte(yamlContent), &node)
	assert.NoError(t, err)

	// Process the custom tags.
	err = processCustomTags(nil, &node, "")
	assert.NoError(t, err)

	// After processing, check if the values preserve their structure.
	// This will help us understand where the newlines are being stripped.
}
