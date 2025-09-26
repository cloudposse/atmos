package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

// TestYAMLTagValuePreservation tests how YAML handles values with custom tags.
func TestYAMLTagValuePreservation(t *testing.T) {
	// Test what happens when a YAML node with a custom tag is processed
	yamlContent := `
vars:
  # This simulates what happens after terraform.output returns a multiline value
  test_value: !terraform.output "component stack output"
`

	var node yaml.Node
	err := yaml.Unmarshal([]byte(yamlContent), &node)
	assert.NoError(t, err)

	// Find the node with the custom tag
	var findTaggedNode func(*yaml.Node) *yaml.Node
	findTaggedNode = func(n *yaml.Node) *yaml.Node {
		if n.Tag == "!terraform.output" {
			return n
		}
		for _, child := range n.Content {
			if found := findTaggedNode(child); found != nil {
				return found
			}
		}
		return nil
	}

	taggedNode := findTaggedNode(&node)
	assert.NotNil(t, taggedNode, "Should find the tagged node")
	assert.Equal(t, "!terraform.output", taggedNode.Tag)
	assert.Equal(t, "component stack output", taggedNode.Value)

	// Now simulate what happens when we replace the value with a multiline string
	// This is what would happen after GetTerraformOutput returns
	multilineValue := "bar\nbaz\nbongo\n"

	// When we set a multiline value back to a scalar node
	taggedNode.Value = multilineValue
	taggedNode.Tag = "" // Clear the tag since it's been processed
	taggedNode.Kind = yaml.ScalarNode
	taggedNode.Style = yaml.DoubleQuotedStyle // Try to preserve the value

	// Marshal back to YAML
	output, err := yaml.Marshal(&node)
	assert.NoError(t, err)

	t.Logf("Output YAML:\n%s", string(output))

	// Unmarshal again to see what we get
	var result map[string]any
	err = yaml.Unmarshal(output, &result)
	assert.NoError(t, err)

	vars, ok := result["vars"].(map[string]any)
	assert.True(t, ok)

	testValue, ok := vars["test_value"].(string)
	assert.True(t, ok)
	assert.Equal(t, multilineValue, testValue, "Multiline value should be preserved")
}
