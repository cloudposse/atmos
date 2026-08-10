package dependencies

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/dependency"
)

func TestRender_YAMLStructure(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"vpc": {},
			"app": dependsOn(map[string]any{"component": "vpc"}),
		},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	out, err := Render(graph, Options{Format: "yaml", Direction: DirectionBoth, Component: "app", Stack: "dev"})
	require.NoError(t, err)
	assert.Contains(t, out, "component: app")
	assert.Contains(t, out, "depends_on:")
	assert.Contains(t, out, "required_by:")
	assert.Contains(t, out, "component: vpc")
}

func TestRender_ForwardOnlyOmitsRequiredBy(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"vpc": {},
			"app": dependsOn(map[string]any{"component": "vpc"}),
		},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	out, err := Render(graph, Options{Format: "json", Direction: DirectionForward, Component: "app", Stack: "dev"})
	require.NoError(t, err)
	assert.Contains(t, out, `"depends_on"`)
	assert.NotContains(t, out, `"required_by"`)
}

func TestRender_ReverseOnlyOmitsDependsOn(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"vpc": {},
			"app": dependsOn(map[string]any{"component": "vpc"}),
		},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	// vpc is required_by app; reverse direction should expose required_by only.
	out, err := Render(graph, Options{Format: "json", Direction: DirectionReverse, Component: "vpc", Stack: "dev"})
	require.NoError(t, err)
	assert.Contains(t, out, `"required_by"`)
	assert.NotContains(t, out, `"depends_on"`)
	assert.Contains(t, out, `"component": "app"`)
}

// TestRender_LeafEmitsEmptyArrayNotNull pins the documented contract: an included
// direction with no edges serializes as an empty array (present), while an
// excluded direction is omitted entirely.
func TestRender_LeafEmitsEmptyArrayNotNull(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {"vpc": {}},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	out, err := Render(graph, Options{Format: "json", Direction: DirectionForward, Component: "vpc", Stack: "dev"})
	require.NoError(t, err)

	var entries []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &entries))
	require.Len(t, entries, 1)

	depsOn, ok := entries[0]["depends_on"]
	require.True(t, ok, "depends_on must be present (empty array) for forward direction")
	assert.NotNil(t, depsOn)
	assert.Empty(t, depsOn, "leaf node has an empty depends_on array")

	_, hasReq := entries[0]["required_by"]
	assert.False(t, hasReq, "required_by must be absent for forward-only direction")
}

// TestRefsFor_SortsAndSkipsMissing covers the resolver directly: IDs not present
// in the graph are skipped, and the result is sorted by stack then component.
func TestRefsFor_SortsAndSkipsMissing(t *testing.T) {
	graph := dependency.NewGraph()
	require.NoError(t, graph.AddNode(&dependency.Node{ID: NodeID("z", "dev"), Component: "z", Stack: "dev"}))
	require.NoError(t, graph.AddNode(&dependency.Node{ID: NodeID("a", "dev"), Component: "a", Stack: "dev"}))
	require.NoError(t, graph.AddNode(&dependency.Node{ID: NodeID("m", "prod"), Component: "m", Stack: "prod"}))

	ids := []string{NodeID("z", "dev"), "does-not-exist", NodeID("m", "prod"), NodeID("a", "dev")}
	refs := refsFor(graph, ids)

	require.Len(t, refs, 3, "the missing ID is skipped")
	assert.Equal(t, componentRef{Component: "a", Stack: "dev"}, refs[0])
	assert.Equal(t, componentRef{Component: "z", Stack: "dev"}, refs[1])
	assert.Equal(t, componentRef{Component: "m", Stack: "prod"}, refs[2])
}

func TestRefsFor_EmptyReturnsNonNil(t *testing.T) {
	refs := refsFor(dependency.NewGraph(), nil)
	assert.NotNil(t, refs)
	assert.Empty(t, refs)
}

func TestRenderStructured_InvalidFormat(t *testing.T) {
	_, err := renderStructured(dependency.NewGraph(), nil, Options{Format: "xml"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidFormat)
}
