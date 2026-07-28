package dependencies

import (
	"encoding/json"
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

func TestRender_LevelsShowsShortestForwardDependencyDistance(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"vpc": {},
			"db":  dependsOn(map[string]any{"component": "vpc"}),
			"app": dependsOn(map[string]any{"component": "db"}),
		},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	out, err := Render(graph, Options{Format: "levels", Direction: DirectionForward, Component: "app", Stack: "dev"})
	require.NoError(t, err)
	assert.Contains(t, out, "Level")
	assert.Less(t, strings.Index(out, "app"), strings.Index(out, "db"))
	assert.Less(t, strings.Index(out, "db"), strings.Index(out, "vpc"))
}

func TestRender_LevelsHonorsTagsAndAllLabelsForRoots(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"vpc": withMetadata(map[string]any{
				"labels": map[string]any{"team": "platform"},
			}),
			"app": dependsOn(map[string]any{"component": "vpc"}),
		},
	})
	app := stacks["dev"].(map[string]any)["components"].(map[string]any)["terraform"].(map[string]any)["app"].(map[string]any)
	app["metadata"] = map[string]any{
		"tags":   []any{"application"},
		"labels": map[string]any{"team": "platform", "environment": "test"},
	}
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	for _, opts := range []Options{
		{Format: "levels", Direction: DirectionForward, Tags: []string{"application", "tier-1"}},
		{Format: "levels", Direction: DirectionForward, Labels: map[string]string{"team": "platform", "environment": "test"}},
	} {
		out, err := Render(graph, opts)
		require.NoError(t, err)
		assert.Less(t, strings.Index(out, "app"), strings.Index(out, "vpc"))
	}
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

	all := selectTopNodes(graph, &Selector{})
	assert.Len(t, all, 3)

	byStack := selectTopNodes(graph, &Selector{Stack: "dev"})
	assert.Len(t, byStack, 2)

	byComponent := selectTopNodes(graph, &Selector{Components: []string{"vpc"}})
	assert.Len(t, byComponent, 2)

	single := selectTopNodes(graph, &Selector{Components: []string{"vpc"}, Stack: "prod"})
	require.Len(t, single, 1)
	assert.Equal(t, "vpc", single[0].Component)
	assert.Equal(t, "prod", single[0].Stack)
}

// withMetadata builds a component section holding a metadata subsection.
func withMetadata(metadata map[string]any) map[string]any {
	return map[string]any{"metadata": metadata}
}

func TestSelectTopNodes_TagsLabelsFilters(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"vpc": withMetadata(map[string]any{
				"tags":   []any{"network"},
				"labels": map[string]any{"team": "platform"},
			}),
			"rds": withMetadata(map[string]any{
				"tags": []any{"database"},
			}),
			"bare": {},
		},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	t.Run("tags any-match", func(t *testing.T) {
		tops := selectTopNodes(graph, &Selector{Tags: []string{"network", "database"}})
		require.Len(t, tops, 2)
		assert.Equal(t, "rds", tops[0].Component)
		assert.Equal(t, "vpc", tops[1].Component)
	})

	t.Run("labels all-match", func(t *testing.T) {
		tops := selectTopNodes(graph, &Selector{Labels: map[string]string{"team": "platform"}})
		require.Len(t, tops, 1)
		assert.Equal(t, "vpc", tops[0].Component)

		tops = selectTopNodes(graph, &Selector{Labels: map[string]string{"team": "platform", "env": "dev"}})
		assert.Empty(t, tops)
	})

	t.Run("tags and labels must match the same node", func(t *testing.T) {
		tops := selectTopNodes(graph, &Selector{Tags: []string{"database"}, Labels: map[string]string{"team": "platform"}})
		assert.Empty(t, tops)
	})

	t.Run("combined with stack and component filters", func(t *testing.T) {
		tops := selectTopNodes(graph, &Selector{Components: []string{"vpc"}, Stack: "dev", Tags: []string{"network"}})
		require.Len(t, tops, 1)
		assert.Equal(t, "vpc", tops[0].Component)

		tops = selectTopNodes(graph, &Selector{Components: []string{"rds"}, Stack: "dev", Tags: []string{"network"}})
		assert.Empty(t, tops)
	})

	t.Run("node without metadata never matches active filters", func(t *testing.T) {
		tops := selectTopNodes(graph, &Selector{Components: []string{"bare"}, Tags: []string{"network"}})
		assert.Empty(t, tops)
	})
}

func TestSelectTopNodes_TemplatedSelectorConservativelyMatches(t *testing.T) {
	// A tags/labels value still containing an unresolved template or Atmos
	// YAML-function marker cannot be judged on a lightweight (unevaluated)
	// graph and must count as a match rather than wrongly excluding the node.
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"templated": withMetadata(map[string]any{
				"tags": []any{"{{ .settings.tag }}"},
			}),
			"yamlfunc": withMetadata(map[string]any{
				"tags": []any{"!env DEPLOY_TIER"},
			}),
		},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	tops := selectTopNodes(graph, &Selector{Tags: []string{"anything"}})
	require.Len(t, tops, 2)
	assert.ElementsMatch(t, []string{"templated", "yamlfunc"},
		[]string{tops[0].Component, tops[1].Component})
}

func TestSelectTopNodes_RequestedTemplatedLabelIgnoresUnrelatedUnresolvedLabel(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"service": {
				"vars": map[string]any{"tier": "worker"},
				"metadata": map[string]any{
					"tags": []any{"active"},
					"labels": map[string]any{
						"tier":   "{{ .vars.tier }}",
						"broken": "{{ unknownFunction }}",
					},
				},
			},
		},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	tops := selectTopNodes(graph, &Selector{
		Tags:   []string{"active"},
		Labels: map[string]string{"tier": "api"},
	})
	assert.Empty(t, tops, "an unrelated unresolved label must not make a requested label undecidable")
}

func TestRender_TagsFilterScopesTopEntries(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"vpc": withMetadata(map[string]any{"tags": []any{"network"}}),
			"app": dependsOn(map[string]any{"component": "vpc"}),
		},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	out, err := Render(graph, Options{Format: "json", Direction: DirectionBoth, Tags: []string{"network"}})
	require.NoError(t, err)

	// vpc is the only top-level entry; its subtree still reaches app as a dependent.
	var entries []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &entries))
	require.Len(t, entries, 1)
	assert.Equal(t, "vpc", entries[0]["component"])
	assert.Contains(t, out, `"app"`, "app must still appear inside vpc's dependents subtree")
}
