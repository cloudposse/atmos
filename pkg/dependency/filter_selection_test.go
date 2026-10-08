package dependency

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/config"
)

// buildSelectionGraph builds a graph from node IDs and (from, to, optional) edges where from depends on to.
func buildSelectionGraph(t *testing.T, ids []string, edges [][3]any) *Graph {
	t.Helper()
	graph := NewGraph()
	for _, id := range ids {
		require.NoError(t, graph.AddNode(&Node{ID: id, Component: id, Stack: "dev", Type: config.TerraformComponentType}))
	}
	for _, edge := range edges {
		require.NoError(t, graph.AddDependencyWithOptional(edge[0].(string), edge[1].(string), edge[2].(bool)))
	}
	graph.IdentifyRoots()
	return graph
}

// sortedNodeIDs returns the IDs of the graph nodes in sorted order.
func sortedNodeIDs(graph *Graph) []string {
	ids := make([]string, 0, len(graph.Nodes))
	for id := range graph.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// sortedCopy returns a sorted copy of the input without modifying it.
func sortedCopy(in []string) []string {
	out := append([]string{}, in...)
	sort.Strings(out)
	return out
}

// keepByID returns a predicate that keeps only the nodes whose ID is in the allowed set.
func keepByID(allowed ...string) func(*Node) bool {
	set := make(map[string]bool, len(allowed))
	for _, id := range allowed {
		set[id] = true
	}
	return func(n *Node) bool { return set[n.ID] }
}

// TestGraph_FilterSelection_EqualsFilterWithoutSelectors proves the default (no selector) path is identical to Filter.
func TestGraph_FilterSelection_EqualsFilterWithoutSelectors(t *testing.T) {
	chain := buildSelectionGraph(t, []string{"a", "b", "c", "d"}, [][3]any{
		{"a", "b", false}, {"b", "c", true}, {"c", "d", false},
	})
	diamond := buildSelectionGraph(t, []string{"top", "left", "bottom", "under"}, [][3]any{
		{"top", "left", false}, {"left", "bottom", false}, {"top", "bottom", true}, {"bottom", "under", false},
	})
	cycle := buildSelectionGraph(t, []string{"x", "y", "z"}, [][3]any{
		{"x", "y", false}, {"y", "x", false}, {"y", "z", true},
	})

	tests := []struct {
		name   string
		graph  *Graph
		filter Filter
	}{
		{"chain seeds only", chain, Filter{NodeIDs: []string{"b"}}},
		{"chain deps", chain, Filter{NodeIDs: []string{"a"}, IncludeDependencies: true}},
		{"chain dependents", chain, Filter{NodeIDs: []string{"c"}, IncludeDependents: true}},
		{"chain both with depth", chain, Filter{NodeIDs: []string{"b"}, IncludeDependencies: true, IncludeDependents: true, DependencyDepth: 1, DependentDepth: 1}},
		{"diamond dependents", diamond, Filter{NodeIDs: []string{"under"}, IncludeDependents: true}},
		{"diamond depth", diamond, Filter{NodeIDs: []string{"top", "under"}, IncludeDependencies: true, DependencyDepth: 2}},
		{"cycle both", cycle, Filter{NodeIDs: []string{"y"}, IncludeDependencies: true, IncludeDependents: true}},
		{"missing seed ignored", chain, Filter{NodeIDs: []string{"nope", "a"}, IncludeDependencies: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.graph.Filter(tt.filter)
			got := tt.graph.FilterSelection(&SelectionFilter{
				Seeds:               tt.filter.NodeIDs,
				IncludeDependencies: tt.filter.IncludeDependencies,
				DependencyDepth:     tt.filter.DependencyDepth,
				IncludeDependents:   tt.filter.IncludeDependents,
				DependentDepth:      tt.filter.DependentDepth,
			})

			require.Equal(t, sortedNodeIDs(want), sortedNodeIDs(got))
			for id, wantNode := range want.Nodes {
				gotNode := got.Nodes[id]
				assert.Equal(t, wantNode.Dependencies, gotNode.Dependencies, "dependencies of %s", id)
				assert.Equal(t, wantNode.Dependents, gotNode.Dependents, "dependents of %s", id)
				assert.Equal(t, wantNode.OptionalDependencies, gotNode.OptionalDependencies, "optional of %s", id)
			}
			assert.ElementsMatch(t, want.Roots, got.Roots)
		})
	}
}

// TestGraph_FilterSelection_Contraction verifies that dropped intermediate nodes are contracted so remaining nodes keep their dependency edges.
func TestGraph_FilterSelection_Contraction(t *testing.T) {
	// leaf -> middle -> base (leaf depends on middle depends on base).
	chain := func(t *testing.T, optionalMiddle, optionalLeaf bool) *Graph {
		return buildSelectionGraph(t, []string{"base", "middle", "leaf"}, [][3]any{
			{"middle", "base", optionalMiddle}, {"leaf", "middle", optionalLeaf},
		})
	}

	t.Run("dropped middle is contracted so leaf still depends on base", func(t *testing.T) {
		got := chain(t, false, false).FilterSelection(&SelectionFilter{
			Seeds:             []string{"base"},
			IncludeDependents: true,
			KeepDependent:     keepByID("leaf"),
		})

		assert.Equal(t, []string{"base", "leaf"}, sortedNodeIDs(got))
		assert.Equal(t, []string{"base"}, got.Nodes["leaf"].Dependencies)
		assert.Equal(t, []string{"leaf"}, got.Nodes["base"].Dependents)
		assert.Empty(t, got.Nodes["base"].Dependencies)
		assert.Equal(t, []string{"base"}, got.Roots)
		assert.False(t, got.Nodes["leaf"].OptionalDependencies["base"])
	})

	t.Run("contracted edge is optional only if every hop is optional", func(t *testing.T) {
		tests := []struct {
			name             string
			middle, leaf     bool
			expectedOptional bool
		}{
			{"all optional", true, true, true},
			{"first hop required", false, true, false},
			{"second hop required", true, false, false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := chain(t, tt.middle, tt.leaf).FilterSelection(&SelectionFilter{
					Seeds:             []string{"base"},
					IncludeDependents: true,
					KeepDependent:     keepByID("leaf"),
				})
				require.Equal(t, []string{"base"}, got.Nodes["leaf"].Dependencies)
				assert.Equal(t, tt.expectedOptional, got.Nodes["leaf"].OptionalDependencies["base"])
			})
		}
	})

	t.Run("required direct edge wins over optional contracted path", func(t *testing.T) {
		graph := buildSelectionGraph(t, []string{"base", "middle", "leaf"}, [][3]any{
			{"middle", "base", true}, {"leaf", "middle", true}, {"leaf", "base", false},
		})
		got := graph.FilterSelection(&SelectionFilter{
			Seeds:             []string{"base"},
			IncludeDependents: true,
			KeepDependent:     keepByID("leaf"),
		})
		assert.Equal(t, []string{"base"}, got.Nodes["leaf"].Dependencies)
		assert.Equal(t, []string{"leaf"}, got.Nodes["base"].Dependents)
		assert.False(t, got.Nodes["leaf"].OptionalDependencies["base"])
	})

	t.Run("two dropped intermediates are contracted through", func(t *testing.T) {
		graph := buildSelectionGraph(t, []string{"base", "m1", "m2", "leaf"}, [][3]any{
			{"m1", "base", false}, {"m2", "m1", false}, {"leaf", "m2", false},
		})
		got := graph.FilterSelection(&SelectionFilter{
			Seeds:             []string{"base"},
			IncludeDependents: true,
			KeepDependent:     keepByID("leaf"),
		})
		assert.Equal(t, []string{"base", "leaf"}, sortedNodeIDs(got))
		assert.Equal(t, []string{"base"}, got.Nodes["leaf"].Dependencies)
	})

	t.Run("cycle inside the dropped region terminates", func(t *testing.T) {
		graph := buildSelectionGraph(t, []string{"base", "m1", "m2", "leaf"}, [][3]any{
			{"m1", "base", false}, {"m1", "m2", false}, {"m2", "m1", false}, {"leaf", "m2", false},
		})
		got := graph.FilterSelection(&SelectionFilter{
			Seeds:             []string{"base"},
			IncludeDependents: true,
			KeepDependent:     keepByID("leaf"),
		})
		assert.Equal(t, []string{"base", "leaf"}, sortedNodeIDs(got))
		assert.Equal(t, []string{"base"}, got.Nodes["leaf"].Dependencies)
	})

	t.Run("kept middle is not contracted", func(t *testing.T) {
		got := chain(t, false, false).FilterSelection(&SelectionFilter{
			Seeds:             []string{"base"},
			IncludeDependents: true,
		})
		assert.Equal(t, []string{"base", "leaf", "middle"}, sortedNodeIDs(got))
		assert.Equal(t, []string{"middle"}, got.Nodes["leaf"].Dependencies)
	})
}

// TestGraph_FilterSelection_DepthCountedThroughDroppedNodes verifies that the dependency depth limit counts levels through dropped nodes.
func TestGraph_FilterSelection_DepthCountedThroughDroppedNodes(t *testing.T) {
	// leaf -> middle -> base.
	graph := buildSelectionGraph(t, []string{"base", "middle", "leaf"}, [][3]any{
		{"middle", "base", false}, {"leaf", "middle", false},
	})

	tests := []struct {
		name     string
		depth    int
		expected []string
	}{
		{"depth 1 stops at dropped middle", 1, []string{"base"}},
		{"depth 2 reaches leaf through dropped middle", 2, []string{"base", "leaf"}},
		{"unlimited reaches leaf", 0, []string{"base", "leaf"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := graph.FilterSelection(&SelectionFilter{
				Seeds:             []string{"base"},
				IncludeDependents: true,
				DependentDepth:    tt.depth,
				KeepDependent:     keepByID("leaf"),
			})
			assert.Equal(t, tt.expected, sortedNodeIDs(got))
		})
	}
}

// TestGraph_FilterSelection_DroppedSeed verifies that a seed rejected by the selector is dropped while matching dependents are kept.
func TestGraph_FilterSelection_DroppedSeed(t *testing.T) {
	// app -> database -> vpc, plus other -> vpc.
	graph := buildSelectionGraph(t, []string{"vpc", "database", "app", "other"}, [][3]any{
		{"database", "vpc", false}, {"app", "database", false}, {"other", "vpc", false},
	})

	t.Run("non-matching seed is dropped but matching transitive dependent is kept", func(t *testing.T) {
		got := graph.FilterSelection(&SelectionFilter{
			Seeds:             nil,
			DependentSeeds:    []string{"vpc"},
			IncludeDependents: true,
			KeepDependent:     keepByID("app"),
		})
		assert.Equal(t, []string{"app"}, sortedNodeIDs(got))
		assert.Empty(t, got.Nodes["app"].Dependencies)
		assert.Equal(t, []string{"app"}, got.Roots)
	})

	t.Run("kept seed with dependencies keeps ordering to contracted dependent", func(t *testing.T) {
		got := graph.FilterSelection(&SelectionFilter{
			Seeds:               []string{"vpc"},
			DependentSeeds:      []string{"vpc"},
			IncludeDependencies: true,
			IncludeDependents:   true,
			KeepDependent:       keepByID("app"),
		})
		assert.Equal(t, []string{"app", "vpc"}, sortedNodeIDs(got))
		assert.Equal(t, []string{"vpc"}, got.Nodes["app"].Dependencies)
		assert.Equal(t, []string{"app"}, got.Nodes["vpc"].Dependents)
	})

	t.Run("dependencies of a kept seed are not filtered by KeepDependent", func(t *testing.T) {
		got := graph.FilterSelection(&SelectionFilter{
			Seeds:               []string{"app"},
			DependentSeeds:      []string{"app"},
			IncludeDependencies: true,
			IncludeDependents:   true,
			KeepDependent:       keepByID(),
		})
		assert.Equal(t, []string{"app", "database", "vpc"}, sortedCopy(sortedNodeIDs(got)))
	})

	t.Run("empty non-nil DependentSeeds expands no dependents", func(t *testing.T) {
		got := graph.FilterSelection(&SelectionFilter{
			Seeds:             []string{"vpc"},
			DependentSeeds:    []string{},
			IncludeDependents: true,
		})
		assert.Equal(t, []string{"vpc"}, sortedNodeIDs(got))
	})

	t.Run("without IncludeDependents a dropped seed contributes nothing", func(t *testing.T) {
		got := graph.FilterSelection(&SelectionFilter{
			Seeds:          []string{"vpc"},
			DependentSeeds: []string{"vpc", "database"},
			KeepDependent:  keepByID("app"),
		})
		assert.Equal(t, []string{"vpc"}, sortedNodeIDs(got))
	})
}
