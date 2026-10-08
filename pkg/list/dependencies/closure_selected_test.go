package dependencies

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/dependency"
)

// selectedClosureStacks builds core/vpc (network) <- dev/db (data) <- dev/app (app)
// plus an unrelated dev/worker (app) that depends directly on core/vpc.
func selectedClosureStacks() map[string]any {
	tagged := func(tag string, sections map[string]any) map[string]any {
		section := map[string]any{"metadata": map[string]any{"tags": []any{tag}}}
		for key, value := range sections {
			section[key] = value
		}
		return section
	}
	return terraformStacks(map[string]map[string]map[string]any{
		"core": {
			"vpc": tagged("network", nil),
		},
		"dev": {
			"db":     tagged("data", dependsOn(map[string]any{"component": "vpc", "stack": "core"})),
			"app":    tagged("app", dependsOn(map[string]any{"component": "db"})),
			"worker": tagged("app", dependsOn(map[string]any{"component": "vpc", "stack": "core"})),
		},
	})
}

func sortedClosureNodeIDs(graph *dependency.Graph) []string {
	ids := make([]string, 0, len(graph.Nodes))
	for id := range graph.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func TestSelectedClosure(t *testing.T) {
	t.Parallel()

	graph, err := BuildGraph(selectedClosureStacks())
	require.NoError(t, err)

	vpc, db, app, worker := NodeID("vpc", "core"), NodeID("db", "dev"), NodeID("app", "dev"), NodeID("worker", "dev")

	tests := []struct {
		name      string
		sel       *Selector
		direction Direction
		depths    Depths
		want      []string
	}{
		{
			name:      "dependents honour tags",
			sel:       &Selector{Tags: []string{"network"}},
			direction: DirectionReverse,
			want:      []string{vpc},
		},
		{
			name:      "non-matching seed still reaches matching dependents through dropped intermediates",
			sel:       &Selector{Stack: "core", Tags: []string{"app"}},
			direction: DirectionReverse,
			want:      []string{app, worker},
		},
		{
			name:      "depth is counted through dropped intermediates",
			sel:       &Selector{Stack: "core", Tags: []string{"app"}},
			direction: DirectionReverse,
			depths:    Depths{Dependents: 1},
			want:      []string{worker},
		},
		{
			name:      "prerequisites ignore tags",
			sel:       &Selector{Tags: []string{"app"}, Components: []string{"app"}},
			direction: DirectionForward,
			want:      []string{vpc, db, app},
		},
		{
			name:      "both directions keep unfiltered prerequisites and filtered dependents",
			sel:       &Selector{Components: []string{"db"}, Tags: []string{"data"}},
			direction: DirectionBoth,
			want:      []string{vpc, db},
		},
		{
			name:      "no selectors equals the plain reverse closure",
			sel:       &Selector{Stack: "core"},
			direction: DirectionReverse,
			want:      []string{vpc, db, app, worker},
		},
		{
			name:      "nothing selected yields an empty closure",
			sel:       &Selector{Stack: "nonexistent", Tags: []string{"app"}},
			direction: DirectionBoth,
			want:      []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			closure := SelectedClosure(graph, tt.sel, tt.direction, tt.depths)
			want := append([]string{}, tt.want...)
			sort.Strings(want)
			assert.Equal(t, want, sortedClosureNodeIDs(closure))
		})
	}

	t.Run("contraction keeps app ordered after nothing when the whole chain between is dropped", func(t *testing.T) {
		t.Parallel()

		// db is dropped (tag data); app depends on db, which depended on vpc.
		closure := SelectedClosure(graph, &Selector{Tags: []string{"network", "app"}}, DirectionReverse, Depths{})
		assert.Equal(t, []string{app, vpc, worker}, sortedClosureNodeIDs(closure))
		appNode := closure.Nodes[app]
		require.NotNil(t, appNode)
		assert.Equal(t, []string{vpc}, appNode.Dependencies, "app must still be ordered after vpc through the dropped db")
	})

	t.Run("nil inputs return an empty graph", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, 0, SelectedClosure(nil, &Selector{}, DirectionBoth, Depths{}).Size())
		assert.Equal(t, 0, SelectedClosure(graph, nil, DirectionBoth, Depths{}).Size())
	})
}

// TestSelectedClosureEqualsReachableClosureWithoutSelectors proves the default
// path is unchanged when no tags/labels are requested.
func TestSelectedClosureEqualsReachableClosureWithoutSelectors(t *testing.T) {
	t.Parallel()

	graph, err := BuildGraph(selectedClosureStacks())
	require.NoError(t, err)

	for _, direction := range []Direction{DirectionForward, DirectionReverse, DirectionBoth} {
		for _, stack := range []string{"core", "dev"} {
			sel := &Selector{Stack: stack}
			want := ReachableClosure(graph, Roots(graph, sel), direction, Depths{})
			got := SelectedClosure(graph, sel, direction, Depths{})
			assert.Equal(t, sortedClosureNodeIDs(want), sortedClosureNodeIDs(got), "direction=%s stack=%s", direction, stack)
		}
	}
}

// TestResolveScopedClosureEvaluatesDroppedDependents proves a dependent dropped
// by the tags selector is still evaluated (the executor needs it described to
// order the survivors) while the reported closure excludes it.
func TestResolveScopedClosureEvaluatesDroppedDependents(t *testing.T) {
	t.Parallel()

	fake := &fakeDescribe{full: selectedClosureStacks()}
	result, err := ResolveScopedClosure(fake.describe, &ScopeRequest{
		Stack:            "core",
		Tags:             []string{"app"},
		Direction:        DirectionReverse,
		ProcessTemplates: true,
	})
	require.NoError(t, err)

	assert.Equal(t, []string{NodeID("app", "dev"), NodeID("worker", "dev")}, sortedClosureNodeIDs(result.Closure))

	devComponents := terraformComponentsOf(t, result.Stacks, "dev")
	assert.ElementsMatch(t, []string{"app", "db", "worker"}, devComponents, "the dropped db intermediate must stay described")
	assert.Contains(t, terraformComponentsOf(t, result.Stacks, "core"), "vpc", "the unfiltered seed must stay described")
}

func terraformComponentsOf(t *testing.T, stacks map[string]any, stack string) []string {
	t.Helper()

	stackMap, ok := stacks[stack].(map[string]any)
	require.True(t, ok, "stack %s must be present", stack)
	components, _ := stackMap["components"].(map[string]any)
	terraform, _ := components["terraform"].(map[string]any)
	names := make([]string, 0, len(terraform))
	for name := range terraform {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
