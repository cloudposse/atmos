package dependencies

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/dependency"
)

func TestRootNodeIDs(t *testing.T) {
	t.Parallel()

	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev":  {"app": {}, "vpc": {}},
		"prod": {"app": {}},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	t.Run("empty_filter_matches_everything", func(t *testing.T) {
		t.Parallel()
		ids := Roots(graph, &Selector{})
		assert.ElementsMatch(t, []string{
			NodeID("app", "dev"), NodeID("vpc", "dev"), NodeID("app", "prod"),
		}, ids)
	})

	t.Run("stack_filter", func(t *testing.T) {
		t.Parallel()
		ids := Roots(graph, &Selector{Stack: "dev"})
		assert.ElementsMatch(t, []string{NodeID("app", "dev"), NodeID("vpc", "dev")}, ids)
	})

	t.Run("component_filter", func(t *testing.T) {
		t.Parallel()
		ids := Roots(graph, &Selector{Components: []string{"app"}})
		assert.ElementsMatch(t, []string{NodeID("app", "dev"), NodeID("app", "prod")}, ids)
	})

	t.Run("combined_filter", func(t *testing.T) {
		t.Parallel()
		ids := Roots(graph, &Selector{Components: []string{"app"}, Stack: "prod"})
		assert.Equal(t, []string{NodeID("app", "prod")}, ids)
	})
}

func TestRootNodeIDs_TagsLabels(t *testing.T) {
	t.Parallel()

	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"vpc": withMetadata(map[string]any{
				"tags":   []any{"network"},
				"labels": map[string]any{"team": "platform"},
			}),
			"rds": withMetadata(map[string]any{"tags": []any{"database"}}),
			"templated": withMetadata(map[string]any{
				"tags": []any{"{{ .settings.tag }}"},
			}),
		},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	t.Run("tags_filter", func(t *testing.T) {
		t.Parallel()
		// templated is conservatively included: its tags cannot be judged
		// on a lightweight (unevaluated) graph.
		ids := Roots(graph, &Selector{Tags: []string{"network"}})
		assert.ElementsMatch(t, []string{NodeID("vpc", "dev"), NodeID("templated", "dev")}, ids)
	})

	t.Run("labels_filter", func(t *testing.T) {
		t.Parallel()
		ids := Roots(graph, &Selector{Labels: map[string]string{"team": "platform"}})
		assert.ElementsMatch(t, []string{NodeID("vpc", "dev"), NodeID("templated", "dev")}, ids)
	})
}

// TestReachableClosure_Forward verifies a forward closure includes the root plus
// its transitive dependencies, and excludes a component that depends ON the
// root (a dependent, not a dependency) as well as any fully unrelated component.
func TestReachableClosure_Forward(t *testing.T) {
	t.Parallel()

	// routing -> slot -> account   (chain of dependencies)
	// unrelated                    (no edge to/from the chain)
	// consumer -> routing          (a dependent of routing, not a dependency)
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"account":   {},
			"slot":      dependsOn(map[string]any{"component": "account"}),
			"routing":   dependsOn(map[string]any{"component": "slot"}),
			"consumer":  dependsOn(map[string]any{"component": "routing"}),
			"unrelated": {},
		},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	roots := []string{NodeID("routing", "dev")}
	closure := ReachableClosure(graph, roots, DirectionForward, Depths{})

	assert.ElementsMatch(
		t,
		[]string{NodeID("routing", "dev"), NodeID("slot", "dev"), NodeID("account", "dev")},
		closureNodeIDs(closure),
		"forward closure must include the root's transitive dependencies only",
	)
}

// TestReachableClosure_Reverse verifies a reverse closure includes the root plus
// its transitive dependents, and excludes what the root depends on.
func TestReachableClosure_Reverse(t *testing.T) {
	t.Parallel()

	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"account":   {},
			"slot":      dependsOn(map[string]any{"component": "account"}),
			"routing":   dependsOn(map[string]any{"component": "slot"}),
			"consumer":  dependsOn(map[string]any{"component": "routing"}),
			"unrelated": {},
		},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	roots := []string{NodeID("slot", "dev")}
	closure := ReachableClosure(graph, roots, DirectionReverse, Depths{})

	assert.ElementsMatch(
		t,
		[]string{NodeID("slot", "dev"), NodeID("routing", "dev"), NodeID("consumer", "dev")},
		closureNodeIDs(closure),
		"reverse closure must include the root's transitive dependents only",
	)
}

// TestReachableClosure_Both verifies DirectionBoth (and the "" default) unions
// forward and reverse.
func TestReachableClosure_Both(t *testing.T) {
	t.Parallel()

	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"account":   {},
			"slot":      dependsOn(map[string]any{"component": "account"}),
			"routing":   dependsOn(map[string]any{"component": "slot"}),
			"consumer":  dependsOn(map[string]any{"component": "routing"}),
			"unrelated": {},
		},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	roots := []string{NodeID("slot", "dev")}
	want := []string{
		NodeID("slot", "dev"), NodeID("account", "dev"),
		NodeID("routing", "dev"), NodeID("consumer", "dev"),
	}

	both := ReachableClosure(graph, roots, DirectionBoth, Depths{})
	assert.ElementsMatch(t, want, closureNodeIDs(both), "DirectionBoth must union forward and reverse")

	defaulted := ReachableClosure(graph, roots, Direction(""), Depths{})
	assert.ElementsMatch(t, want, closureNodeIDs(defaulted), "empty Direction must behave like DirectionBoth")
}

// TestReachableClosure_CrossStack verifies dependency edges spanning stacks are
// followed: a root in one stack depending on a component in another stack pulls
// that other stack's component into the closure, while a third, disconnected
// stack is excluded — this is the core customer scenario (bounded root plus its
// cross-stack prerequisite closure).
func TestReachableClosure_CrossStack(t *testing.T) {
	t.Parallel()

	stacks := terraformStacks(map[string]map[string]map[string]any{
		"app-account": {"kms": {}},
		"app-slot1": {
			"eks": dependsOn(map[string]any{"component": "kms", "stack": "app-account"}),
		},
		"app-slot2-in-other-account": {"vpc": {}}, // fully unrelated stack.
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	roots := []string{NodeID("eks", "app-slot1")}
	closure := ReachableClosure(graph, roots, DirectionForward, Depths{})

	ids := closureNodeIDs(closure)
	assert.ElementsMatch(t, []string{NodeID("eks", "app-slot1"), NodeID("kms", "app-account")}, ids)
	assert.NotContains(t, ids, NodeID("vpc", "app-slot2-in-other-account"),
		"an unrelated stack with no edge to the root must be excluded from the closure")
}

// TestReachableClosure_ToleratesCycles verifies a cyclic dependency graph does
// not cause infinite recursion and the closure still contains every node on the
// cycle reachable from the root.
func TestReachableClosure_ToleratesCycles(t *testing.T) {
	t.Parallel()

	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"a": dependsOn(map[string]any{"component": "b"}),
			"b": dependsOn(map[string]any{"component": "a"}),
		},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	roots := []string{NodeID("a", "dev")}
	closure := ReachableClosure(graph, roots, DirectionForward, Depths{})

	assert.ElementsMatch(t, []string{NodeID("a", "dev"), NodeID("b", "dev")}, closureNodeIDs(closure))
}

func TestReachableClosure_EmptyRootsReturnsEmptyGraph(t *testing.T) {
	t.Parallel()

	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {"app": {}},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	closure := ReachableClosure(graph, nil, DirectionBoth, Depths{})
	assert.Equal(t, 0, closure.Size())
}

func TestStackNames(t *testing.T) {
	t.Parallel()

	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev":  {"app": {}, "vpc": {}},
		"prod": {"app": {}},
	})
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	// Assert exact order (not just ElementsMatch) since graph.Nodes is a map;
	// StackNames must sort so results are deterministic across runs.
	assert.Equal(t, []string{"dev", "prod"}, StackNames(graph))
	assert.Nil(t, StackNames(nil))
}

// closureNodeIDs returns the node IDs present in a (typically closure-filtered)
// graph, for order-independent assertions via assert.ElementsMatch.
func closureNodeIDs(graph *dependency.Graph) []string {
	ids := make([]string, 0, len(graph.Nodes))
	for id := range graph.Nodes {
		ids = append(ids, id)
	}
	return ids
}
