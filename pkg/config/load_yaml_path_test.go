package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goyaml "go.yaml.in/yaml/v3"
)

func TestFindYAMLMappingPath(t *testing.T) {
	const doc = `
auth:
  identities:
    prod-admin:
      kind: aws
  providers: scalar-not-a-mapping
top: value
`

	parse := func(t *testing.T) *goyaml.Node {
		t.Helper()
		var node goyaml.Node
		require.NoError(t, goyaml.Unmarshal([]byte(doc), &node))
		return &node
	}

	t.Run("nil node returns nil", func(t *testing.T) {
		assert.Nil(t, findYAMLMappingPath(nil, "auth"))
	})

	t.Run("empty document returns nil", func(t *testing.T) {
		node := &goyaml.Node{Kind: goyaml.DocumentNode}
		assert.Nil(t, findYAMLMappingPath(node, "auth"))
	})

	t.Run("empty path returns the document's root node", func(t *testing.T) {
		node := parse(t)
		got := findYAMLMappingPath(node)
		require.NotNil(t, got)
		assert.Equal(t, goyaml.MappingNode, got.Kind)
	})

	t.Run("single-level key hit", func(t *testing.T) {
		node := parse(t)
		got := findYAMLMappingPath(node, "auth")
		require.NotNil(t, got)
		assert.Equal(t, goyaml.MappingNode, got.Kind)
	})

	t.Run("multi-level key hit", func(t *testing.T) {
		node := parse(t)
		got := findYAMLMappingPath(node, "auth", "identities")
		require.NotNil(t, got)
		require.Equal(t, goyaml.MappingNode, got.Kind)
		// The identities mapping should contain the prod-admin key.
		require.GreaterOrEqual(t, len(got.Content), 2)
		assert.Equal(t, "prod-admin", got.Content[0].Value)
	})

	t.Run("missing key returns nil", func(t *testing.T) {
		node := parse(t)
		assert.Nil(t, findYAMLMappingPath(node, "auth", "missing"))
	})

	t.Run("path through a scalar returns nil", func(t *testing.T) {
		node := parse(t)
		// auth.providers is a scalar, so descending past it must return nil.
		assert.Nil(t, findYAMLMappingPath(node, "auth", "providers", "anything"))
	})
}
