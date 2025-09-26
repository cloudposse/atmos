package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

// TestYAMLFlowScalarNewlines tests YAML flow scalar handling of newlines.
func TestYAMLFlowScalarNewlines(t *testing.T) {
	// Test 1: Basic parsing of escaped newlines
	yaml1 := `foo: "bar\nbaz\nbongo\n"`
	var data1 map[string]any
	err := yaml.Unmarshal([]byte(yaml1), &data1)
	assert.NoError(t, err)
	assert.Equal(t, "bar\nbaz\nbongo\n", data1["foo"])

	// Test 2: What happens when we re-encode to YAML
	output, err := yaml.Marshal(data1)
	assert.NoError(t, err)
	t.Logf("Re-encoded YAML:\n%s", string(output))

	// Test 3: Re-parse and check
	var data2 map[string]any
	err = yaml.Unmarshal(output, &data2)
	assert.NoError(t, err)
	assert.Equal(t, "bar\nbaz\nbongo\n", data2["foo"], "Newlines should be preserved through re-encoding")

	// Test 4: What happens with a tagged scalar
	yaml3 := `foo: !custom "bar\nbaz\nbongo\n"`
	var node yaml.Node
	err = yaml.Unmarshal([]byte(yaml3), &node)
	assert.NoError(t, err)

	// Find the tagged node
	var findTagged func(*yaml.Node) *yaml.Node
	findTagged = func(n *yaml.Node) *yaml.Node {
		if n.Tag == "!custom" {
			return n
		}
		for _, child := range n.Content {
			if found := findTagged(child); found != nil {
				return found
			}
		}
		return nil
	}

	taggedNode := findTagged(&node)
	assert.NotNil(t, taggedNode)

	// Check if the value preserves escapes or converts them
	t.Logf("Tagged node value: %q", taggedNode.Value)

	// When a tagged scalar has escaped sequences, they should be interpreted
	// But the actual behavior depends on how the parser handles it
}
