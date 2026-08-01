package dependencies

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/dependency"
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

// TestRender_InvalidDirectionPropagatesError verifies Render surfaces the
// normalizeDirection error for an unrecognized direction instead of silently
// falling back to a default.
func TestRender_InvalidDirectionPropagatesError(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {"vpc": {}},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	_, err = Render(graph, Options{Format: "tree", Direction: "sideways"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidFlag)
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

	out, err := Render(graph, Options{Format: FormatLevels, Direction: DirectionForward, Component: "app", Stack: "dev"})
	require.NoError(t, err)
	assertLevel(t, out, "app", 0)
	assertLevel(t, out, "db", 1)
	assertLevel(t, out, "vpc", 2)
}

func TestRender_LevelsHonorsTagsAndAllLabelsForRoots(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"vpc": withMetadata(map[string]any{
				"labels": map[string]any{"team": "platform"},
			}),
			"app": {
				"metadata": map[string]any{
					"tags":   []any{"application"},
					"labels": map[string]any{"team": "platform", "environment": "test"},
				},
				"settings": map[string]any{"depends_on": map[string]any{"1": map[string]any{"component": "vpc"}}},
			},
			"ignored": withMetadata(map[string]any{
				"tags":   []any{"other"},
				"labels": map[string]any{"team": "platform", "environment": "other"},
			}),
		},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	tests := []struct {
		name string
		opts Options
	}{
		{"tags_root", Options{Format: FormatLevels, Direction: DirectionForward, Tags: []string{"application", "tier-1"}}},
		{"labels_root", Options{Format: FormatLevels, Direction: DirectionForward, Labels: map[string]string{"team": "platform", "environment": "test"}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Render(graph, tc.opts)
			require.NoError(t, err)
			assertLevel(t, out, "app", 0)
			assertLevel(t, out, "vpc", 1)
			assert.NotContains(t, out, "ignored")
		})
	}
}

// TestRender_LevelsReverseAndBothDirections covers levelNeighbors' reverse
// (dependents only) and both (union) branches; TestRender_LevelsShowsShortestForwardDependencyDistance
// already covers the forward branch.
func TestRender_LevelsReverseAndBothDirections(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"vpc": {},
			"db":  dependsOn(map[string]any{"component": "vpc"}),
			"app": dependsOn(map[string]any{"component": "db"}),
		},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	t.Run("reverse_shows_dependent_distance", func(t *testing.T) {
		out, err := Render(graph, Options{Format: FormatLevels, Direction: DirectionReverse, Component: "vpc", Stack: "dev"})
		require.NoError(t, err)
		assertLevel(t, out, "vpc", 0)
		assertLevel(t, out, "db", 1)
		assertLevel(t, out, "app", 2)
	})

	t.Run("both_unions_forward_and_reverse", func(t *testing.T) {
		out, err := Render(graph, Options{Format: FormatLevels, Direction: DirectionBoth, Component: "db", Stack: "dev"})
		require.NoError(t, err)
		assertLevel(t, out, "db", 0)
		assertLevel(t, out, "vpc", 1)
		assertLevel(t, out, "app", 1)
	})
}

// TestRender_LevelsDiamondConvergesToShortestDistance verifies levelDistances
// visits a node reachable via two paths only once, at its shortest distance —
// the BFS-dedup branch (a node already leveled by a shorter path is skipped
// when reached again by a longer one).
func TestRender_LevelsDiamondConvergesToShortestDistance(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"top":    dependsOn(map[string]any{"component": "left"}, map[string]any{"component": "bottom"}),
			"left":   dependsOn(map[string]any{"component": "bottom"}),
			"bottom": {},
		},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	out, err := Render(graph, Options{Format: FormatLevels, Direction: DirectionForward, Component: "top", Stack: "dev"})
	require.NoError(t, err)
	assertLevel(t, out, "top", 0)
	assertLevel(t, out, "left", 1)
	// bottom is reachable at depth 1 (top->bottom) and depth 2 (top->left->bottom);
	// the shorter path must win.
	assertLevel(t, out, "bottom", 1)
}

// TestSortedLevelNodes_TieBreaksByStackThenComponent covers the sort
// comparator's tie-break branches directly: same level with different
// stacks, and same level+stack with different components.
func TestSortedLevelNodes_TieBreaksByStackThenComponent(t *testing.T) {
	graph := dependency.NewGraph()
	nodes := []*dependency.Node{
		{ID: "z-dev", Component: "z", Stack: "dev"},
		{ID: "a-dev", Component: "a", Stack: "dev"},
		{ID: "m-prod", Component: "m", Stack: "prod"},
	}
	for _, n := range nodes {
		require.NoError(t, graph.AddNode(n))
	}

	// All at the same level (0): must sort by stack, then by component.
	levels := map[string]int{"z-dev": 0, "a-dev": 0, "m-prod": 0}
	sorted := sortedLevelNodes(graph, levels)
	require.Len(t, sorted, 3)
	assert.Equal(t, "dev", sorted[0].Stack)
	assert.Equal(t, "a", sorted[0].Component, "within the same stack, component breaks the tie")
	assert.Equal(t, "dev", sorted[1].Stack)
	assert.Equal(t, "z", sorted[1].Component)
	assert.Equal(t, "prod", sorted[2].Stack, "a different stack sorts after 'dev' alphabetically")
}

// TestLevelDistances_SkipsUnresolvableNodeID covers the defensive guard in
// levelDistances' BFS loop: an ID present in the frontier but absent from the
// graph (e.g. a node manually referenced but never added) is skipped rather
// than dereferenced.
func TestLevelDistances_SkipsUnresolvableNodeID(t *testing.T) {
	graph := dependency.NewGraph()
	require.NoError(t, graph.AddNode(&dependency.Node{ID: "real", Component: "real", Stack: "dev"}))

	ghost := &dependency.Node{ID: "ghost", Component: "ghost", Stack: "dev"} // never added to graph.
	levels := levelDistances(graph, []*dependency.Node{ghost}, DirectionForward)

	// The ghost seed itself is still recorded at level 0 (levels seeding happens
	// before the graph lookup), but BFS expansion from it must not panic and
	// must not resolve any node reachable through it.
	assert.Equal(t, map[string]int{"ghost": 0}, levels)
}

// TestLevelDistances_DedupesDuplicateTopNodes covers the "already seeded"
// branch when the same node appears twice in tops (defensive de-duplication;
// selectTopNodes itself never produces duplicates since it iterates a map,
// but levelDistances must still tolerate a caller passing one).
func TestLevelDistances_DedupesDuplicateTopNodes(t *testing.T) {
	graph := dependency.NewGraph()
	node := &dependency.Node{ID: "dup", Component: "dup", Stack: "dev"}
	require.NoError(t, graph.AddNode(node))

	levels := levelDistances(graph, []*dependency.Node{node, node}, DirectionForward)
	assert.Equal(t, map[string]int{"dup": 0}, levels)
}

// TestNodeMatchesTagsLabels_ResolvesTemplatedTag verifies a templated tag
// value that resolves cleanly is matched against the filter using its
// resolved value, not conservatively counted as a match by default.
func TestNodeMatchesTagsLabels_ResolvesTemplatedTag(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"app": {
				"vars": map[string]any{"tier": "database"},
				"metadata": map[string]any{
					"tags": []any{"{{ .vars.tier }}"},
				},
			},
		},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	// The template resolves to "database", which is in the filter: matches.
	tops := selectTopNodes(graph, &Selector{Tags: []string{"database"}})
	require.Len(t, tops, 1)
	assert.Equal(t, "app", tops[0].Component)

	// The template still resolves to "database", which is NOT in this filter:
	// a resolved (not conservative) match correctly excludes it.
	tops = selectTopNodes(graph, &Selector{Tags: []string{"network"}})
	assert.Empty(t, tops, "a cleanly resolved tag must be matched exactly, not conservatively")
}

// TestNodeMatchesTagsLabels_UnresolvableLabelConservativelyMatches verifies a
// templated label that fails to resolve (missing key) falls back to a
// conservative match rather than being excluded.
func TestNodeMatchesTagsLabels_UnresolvableLabelConservativelyMatches(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"app": {
				"metadata": map[string]any{
					"tags":   []any{"active"},
					"labels": map[string]any{"tier": "{{ .vars.missing }}"},
				},
			},
		},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	tops := selectTopNodes(graph, &Selector{Tags: []string{"active"}, Labels: map[string]string{"tier": "anything"}})
	require.Len(t, tops, 1, "an unresolvable requested label must conservatively match rather than exclude")
	assert.Equal(t, "app", tops[0].Component)
}

// TestBuildSubtree_SkipsGhostNeighborID covers the defensive guard in
// buildSubtree: a Dependencies/Dependents entry pointing at an ID no longer
// present in the graph is skipped rather than dereferenced.
func TestBuildSubtree_SkipsGhostNeighborID(t *testing.T) {
	graph := dependency.NewGraph()
	require.NoError(t, graph.AddNode(&dependency.Node{ID: "root", Component: "root", Stack: "dev"}))
	root, _ := graph.GetNode("root")
	// Inject a dangling dependency reference the graph never validated (real
	// edges always point at existing nodes; this simulates a stale reference).
	root.Dependencies = append(root.Dependencies, "ghost")

	children := buildSubtree(graph, root, forward, map[string]bool{"root": true})
	assert.Empty(t, children, "a dangling neighbor ID must be skipped, not surfaced as a child")
}

func assertLevel(t *testing.T, output, component string, want int) {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[2] == component {
			assert.Equal(t, strconv.Itoa(want), fields[0])
			return
		}
	}
	assert.Failf(t, "component missing from levels output", "%q not found in:\n%s", component, output)
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
