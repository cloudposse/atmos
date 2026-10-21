package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/function/yamlsource"
)

func TestConvertToYAMLPreservesSyntaxTree(t *testing.T) {
	const input = "# heading\nvalue: &value !example text # inline\ncopy: *value\nrecursive: &recursive [*recursive]\n"
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(input), &node))
	for _, value := range []any{node, &node} {
		output, err := ConvertToYAML(value)
		require.NoError(t, err)
		assert.Equal(t, input, output)
	}
}

func TestConvertToYAMLRestoresTagsWithoutMutatingNode(t *testing.T) {
	value := (yamlsource.Source{Tag: "!env", Argument: "REGION"}).Encode()
	var node yaml.Node
	require.NoError(t, node.Encode(map[string]any{"value": value}))
	output, err := ConvertToYAML(&node)
	require.NoError(t, err)
	assert.Equal(t, "value: !env \"REGION\"\n", output)
	assert.Equal(t, value, node.Content[1].Value)
	assert.Equal(t, "!!str", node.Content[1].Tag)
}
