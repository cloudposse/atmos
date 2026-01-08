package yaml

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goyaml "gopkg.in/yaml.v3"
)

func TestExtractPositions_Disabled(t *testing.T) {
	// When disabled, should return empty map.
	node := &goyaml.Node{}
	positions := ExtractPositions(node, false)
	assert.Empty(t, positions)
}

func TestExtractPositions_NilNode(t *testing.T) {
	// Should handle nil node gracefully.
	positions := ExtractPositions(nil, true)
	assert.Empty(t, positions)
}

func TestExtractPositions_ScalarValue(t *testing.T) {
	yamlContent := `key: value`
	var node goyaml.Node
	err := goyaml.Unmarshal([]byte(yamlContent), &node)
	require.NoError(t, err)

	positions := ExtractPositions(&node, true)

	// Should have position for 'key'.
	assert.True(t, HasPosition(positions, "key"))
	pos := GetPosition(positions, "key")
	assert.Equal(t, 1, pos.Line)
}

func TestExtractPositions_NestedMapping(t *testing.T) {
	yamlContent := `
parent:
  child1: value1
  child2: value2
`
	var node goyaml.Node
	err := goyaml.Unmarshal([]byte(yamlContent), &node)
	require.NoError(t, err)

	positions := ExtractPositions(&node, true)

	// Should have positions for all keys.
	assert.True(t, HasPosition(positions, "parent"))
	assert.True(t, HasPosition(positions, "parent.child1"))
	assert.True(t, HasPosition(positions, "parent.child2"))
}

func TestExtractPositions_Sequence(t *testing.T) {
	yamlContent := `
items:
  - first
  - second
  - third
`
	var node goyaml.Node
	err := goyaml.Unmarshal([]byte(yamlContent), &node)
	require.NoError(t, err)

	positions := ExtractPositions(&node, true)

	// Should have positions for array items.
	assert.True(t, HasPosition(positions, "items"))
	assert.True(t, HasPosition(positions, "items[0]"))
	assert.True(t, HasPosition(positions, "items[1]"))
	assert.True(t, HasPosition(positions, "items[2]"))
}

func TestExtractPositions_MixedContent(t *testing.T) {
	yamlContent := `
metadata:
  name: test
  labels:
    app: myapp
    version: v1
servers:
  - host: server1
    port: 8080
  - host: server2
    port: 9090
`
	var node goyaml.Node
	err := goyaml.Unmarshal([]byte(yamlContent), &node)
	require.NoError(t, err)

	positions := ExtractPositions(&node, true)

	// Test various paths.
	assert.True(t, HasPosition(positions, "metadata"))
	assert.True(t, HasPosition(positions, "metadata.name"))
	assert.True(t, HasPosition(positions, "metadata.labels"))
	assert.True(t, HasPosition(positions, "metadata.labels.app"))
	assert.True(t, HasPosition(positions, "metadata.labels.version"))
	assert.True(t, HasPosition(positions, "servers"))
	assert.True(t, HasPosition(positions, "servers[0]"))
	assert.True(t, HasPosition(positions, "servers[0].host"))
	assert.True(t, HasPosition(positions, "servers[0].port"))
	assert.True(t, HasPosition(positions, "servers[1]"))
	assert.True(t, HasPosition(positions, "servers[1].host"))
	assert.True(t, HasPosition(positions, "servers[1].port"))
}

func TestExtractPositions_AliasNode(t *testing.T) {
	yamlContent := `
defaults: &defaults
  adapter: postgres
  host: localhost

development:
  database: dev_db
  <<: *defaults
`
	var node goyaml.Node
	err := goyaml.Unmarshal([]byte(yamlContent), &node)
	require.NoError(t, err)

	positions := ExtractPositions(&node, true)

	// Should have positions for anchor and alias content.
	assert.True(t, HasPosition(positions, "defaults"))
	assert.True(t, HasPosition(positions, "defaults.adapter"))
	assert.True(t, HasPosition(positions, "development"))
}

func TestGetPosition_NotFound(t *testing.T) {
	positions := PositionMap{
		"exists": Position{Line: 1, Column: 1},
	}

	// Existing path.
	pos := GetPosition(positions, "exists")
	assert.Equal(t, 1, pos.Line)
	assert.Equal(t, 1, pos.Column)

	// Non-existing path returns zero value.
	pos = GetPosition(positions, "not-exists")
	assert.Equal(t, 0, pos.Line)
	assert.Equal(t, 0, pos.Column)
}

func TestGetPosition_NilMap(t *testing.T) {
	pos := GetPosition(nil, "any")
	assert.Equal(t, 0, pos.Line)
	assert.Equal(t, 0, pos.Column)
}

func TestHasPosition_NilMap(t *testing.T) {
	assert.False(t, HasPosition(nil, "any"))
}

func TestHasPosition_ExistsAndNotExists(t *testing.T) {
	positions := PositionMap{
		"exists": Position{Line: 5, Column: 10},
	}

	assert.True(t, HasPosition(positions, "exists"))
	assert.False(t, HasPosition(positions, "not-exists"))
}

func TestExtractPositions_EmptyDocument(t *testing.T) {
	yamlContent := ``
	var node goyaml.Node
	err := goyaml.Unmarshal([]byte(yamlContent), &node)
	require.NoError(t, err)

	positions := ExtractPositions(&node, true)
	assert.Empty(t, positions)
}

func TestExtractPositions_DeeplyNested(t *testing.T) {
	yamlContent := `
a:
  b:
    c:
      d:
        e: deep
`
	var node goyaml.Node
	err := goyaml.Unmarshal([]byte(yamlContent), &node)
	require.NoError(t, err)

	positions := ExtractPositions(&node, true)

	assert.True(t, HasPosition(positions, "a"))
	assert.True(t, HasPosition(positions, "a.b"))
	assert.True(t, HasPosition(positions, "a.b.c"))
	assert.True(t, HasPosition(positions, "a.b.c.d"))
	assert.True(t, HasPosition(positions, "a.b.c.d.e"))
}

func TestExtractPositions_SequenceOfMappings(t *testing.T) {
	yamlContent := `
list:
  - name: item1
    value: 100
  - name: item2
    value: 200
`
	var node goyaml.Node
	err := goyaml.Unmarshal([]byte(yamlContent), &node)
	require.NoError(t, err)

	positions := ExtractPositions(&node, true)

	assert.True(t, HasPosition(positions, "list"))
	assert.True(t, HasPosition(positions, "list[0]"))
	assert.True(t, HasPosition(positions, "list[0].name"))
	assert.True(t, HasPosition(positions, "list[0].value"))
	assert.True(t, HasPosition(positions, "list[1]"))
	assert.True(t, HasPosition(positions, "list[1].name"))
	assert.True(t, HasPosition(positions, "list[1].value"))
}

func TestExtractPositions_LineNumbers(t *testing.T) {
	yamlContent := `first: value1
second: value2
third: value3`
	var node goyaml.Node
	err := goyaml.Unmarshal([]byte(yamlContent), &node)
	require.NoError(t, err)

	positions := ExtractPositions(&node, true)

	// Verify line numbers are correct.
	assert.Equal(t, 1, GetPosition(positions, "first").Line)
	assert.Equal(t, 2, GetPosition(positions, "second").Line)
	assert.Equal(t, 3, GetPosition(positions, "third").Line)
}

func TestPosition_Struct(t *testing.T) {
	pos := Position{
		Line:   10,
		Column: 5,
	}

	assert.Equal(t, 10, pos.Line)
	assert.Equal(t, 5, pos.Column)
}

func TestPositionMap_Type(t *testing.T) {
	// Test that PositionMap works as a map.
	pm := make(PositionMap)
	pm["test"] = Position{Line: 1, Column: 2}

	assert.Equal(t, Position{Line: 1, Column: 2}, pm["test"])
}
