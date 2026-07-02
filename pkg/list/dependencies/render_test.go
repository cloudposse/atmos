package dependencies

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestNormalizeDirection(t *testing.T) {
	tests := []struct {
		in      Direction
		want    Direction
		wantErr bool
	}{
		{"", DirectionBoth, false},
		{DirectionBoth, DirectionBoth, false},
		{DirectionForward, DirectionForward, false},
		{DirectionReverse, DirectionReverse, false},
		{"sideways", "sideways", true},
	}
	for _, tt := range tests {
		opts := Options{Direction: tt.in}
		err := normalizeDirection(&opts)
		if tt.wantErr {
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrInvalidFlag)
			continue
		}
		require.NoError(t, err)
		assert.Equal(t, tt.want, opts.Direction)
	}
}

func TestRender_InvalidFormat(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {"vpc": {}},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	_, err = Render(graph, Options{Format: "xml"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidFormat)
}

func TestRender_TreeForwardChain(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"vpc": {},
			"app": dependsOn(map[string]any{"component": "vpc"}),
			"web": dependsOn(map[string]any{"component": "app"}),
		},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	out, err := Render(graph, Options{Format: "tree", Direction: DirectionForward, Component: "web", Stack: "dev"})
	require.NoError(t, err)

	// web -> app -> vpc should all appear, transitively.
	assert.Contains(t, out, "web")
	assert.Contains(t, out, "app")
	assert.Contains(t, out, "vpc")
}

func TestRender_TreeBothShowsBranches(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"vpc": {},
			"app": dependsOn(map[string]any{"component": "vpc"}),
		},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	out, err := Render(graph, Options{Format: "tree", Direction: DirectionBoth, Component: "app", Stack: "dev"})
	require.NoError(t, err)
	assert.Contains(t, out, "depends on")
	assert.Contains(t, out, "required by")
}

func TestRender_TreeMarksCircular(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"a": dependsOn(map[string]any{"component": "b"}),
			"b": dependsOn(map[string]any{"component": "a"}),
		},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	// Must terminate (cycle guard) and surface the circular marker.
	out, err := Render(graph, Options{Format: "tree", Direction: DirectionForward, Component: "a", Stack: "dev"})
	require.NoError(t, err)
	assert.Contains(t, out, "circular reference")
}

func TestRender_JSONStructure(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"vpc": {},
			"app": dependsOn(map[string]any{"component": "vpc"}),
		},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	out, err := Render(graph, Options{Format: "json", Direction: DirectionBoth, Component: "app", Stack: "dev"})
	require.NoError(t, err)
	assert.True(t, strings.Contains(out, `"component": "app"`))
	assert.True(t, strings.Contains(out, `"depends_on"`))
	assert.True(t, strings.Contains(out, `"component": "vpc"`))
}

func TestSelectTopNodes_Filters(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev":  {"vpc": {}, "app": {}},
		"prod": {"vpc": {}},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	all := selectTopNodes(graph, "", "")
	assert.Len(t, all, 3)

	byStack := selectTopNodes(graph, "", "dev")
	assert.Len(t, byStack, 2)

	byComponent := selectTopNodes(graph, "vpc", "")
	assert.Len(t, byComponent, 2)

	single := selectTopNodes(graph, "vpc", "prod")
	require.Len(t, single, 1)
	assert.Equal(t, "vpc", single[0].Component)
	assert.Equal(t, "prod", single[0].Stack)
}
