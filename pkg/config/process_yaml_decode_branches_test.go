package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

// TestDecodeNodeWithYamlFunctionsBranches covers the error-propagation and default
// (alias) branches of decodeNodeWithYamlFunctions that the happy-path tests do not
// exercise: a failing scalar decode inside a mapping and inside a sequence, and an
// alias node falling through to the default case.
func TestDecodeNodeWithYamlFunctionsBranches(t *testing.T) {
	t.Run("mapping with undecodable scalar returns error", func(t *testing.T) {
		var node yaml.Node
		// "!!int notanint" is a standard (non-Atmos) tag, so it takes the plain
		// decode path and fails, propagating the error up through the mapping loop.
		require.NoError(t, yaml.Unmarshal([]byte("bad: !!int notanint\n"), &node))

		got, err := decodeNodeWithYamlFunctions(&node)
		require.Error(t, err)
		assert.Nil(t, got)
	})

	t.Run("sequence with undecodable scalar returns error", func(t *testing.T) {
		var node yaml.Node
		require.NoError(t, yaml.Unmarshal([]byte("- !!int notanint\n"), &node))

		got, err := decodeNodeWithYamlFunctions(&node)
		require.Error(t, err)
		assert.Nil(t, got)
	})

	t.Run("alias node falls through to default decode", func(t *testing.T) {
		var node yaml.Node
		// The value of "ref" is an alias node (yaml.AliasNode), which is not one of
		// the explicitly handled kinds and so exercises the default branch.
		require.NoError(t, yaml.Unmarshal([]byte("base: &anchor anchored\nref: *anchor\n"), &node))

		got, err := decodeNodeWithYamlFunctions(&node)
		require.NoError(t, err)
		gotMap, ok := got.(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "anchored", gotMap["base"])
		assert.Equal(t, "anchored", gotMap["ref"])
	})
}
