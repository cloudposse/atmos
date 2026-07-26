package dependencies

import (
	"sort"

	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/perf"
)

// RootNodeIDs returns the IDs of nodes matching the given component/stack/
// tags/labels filters, for use as ReachableClosure roots. Mirrors
// selectTopNodes's matching semantics: an empty filter matches every node,
// and a templated (unresolvable) tags/labels selector conservatively matches
// so a lightweight unevaluated graph can never wrongly exclude a root.
func RootNodeIDs(graph *dependency.Graph, component, stack string, tagsFilter []string, labelsFilter map[string]string) []string {
	defer perf.Track(nil, "dependencies.RootNodeIDs")()

	tops := selectTopNodes(graph, component, stack, tagsFilter, labelsFilter)
	ids := make([]string, len(tops))
	for i, node := range tops {
		ids[i] = node.ID
	}
	return ids
}

// ReachableClosure returns the subgraph reachable from roots in the requested
// direction: forward follows Dependencies, reverse follows Dependents, both
// follows either. It reuses Graph.Filter's existing IncludeDependencies/
// IncludeDependents traversal rather than re-implementing graph walking, so
// cross-stack edges (Node.Dependencies/Dependents already span stacks) are
// followed naturally. An empty roots list returns an empty graph.
func ReachableClosure(graph *dependency.Graph, roots []string, direction Direction) *dependency.Graph {
	defer perf.Track(nil, "dependencies.ReachableClosure")()

	if graph == nil || len(roots) == 0 {
		return dependency.NewGraph()
	}

	filter := dependency.Filter{NodeIDs: roots}
	switch direction {
	case DirectionForward:
		filter.IncludeDependencies = true
	case DirectionReverse:
		filter.IncludeDependents = true
	default: // DirectionBoth, or "" before normalizeDirection runs.
		filter.IncludeDependencies = true
		filter.IncludeDependents = true
	}
	return graph.Filter(filter)
}

// StackNames returns the sorted, deduplicated set of stack names present in
// graph. Used to scope a closure-limited full evaluation pass (templates/YAML
// functions/auth) to exactly the stacks a reachable closure touches, instead of
// every stack in the repo.
func StackNames(graph *dependency.Graph) []string {
	defer perf.Track(nil, "dependencies.StackNames")()

	if graph == nil {
		return nil
	}

	seen := make(map[string]struct{}, len(graph.Nodes))
	names := make([]string, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		if _, ok := seen[node.Stack]; ok {
			continue
		}
		seen[node.Stack] = struct{}{}
		names = append(names, node.Stack)
	}
	sort.Strings(names)
	return names
}
