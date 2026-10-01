package dependencies

import (
	"sort"

	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Depths bounds closure expansion per direction, measured in dependency
// levels from the nearest root (0 = unlimited).
type Depths struct {
	// Dependencies bounds forward (what the roots depend on) expansion.
	Dependencies int
	// Dependents bounds reverse (what depends on the roots) expansion.
	Dependents int
}

// ReachableClosure returns the subgraph reachable from roots in the requested
// direction: forward follows Dependencies, reverse follows Dependents, both
// follows either — each bounded by the matching Depths field (0 = unlimited).
// It reuses Graph.Filter's existing IncludeDependencies/IncludeDependents
// traversal rather than re-implementing graph walking, so cross-stack edges
// (Node.Dependencies/Dependents already span stacks) are followed naturally.
// An empty roots list returns an empty graph.
func ReachableClosure(graph *dependency.Graph, roots []string, direction Direction, depths Depths) *dependency.Graph {
	defer perf.Track(nil, "dependencies.ReachableClosure")()

	if graph == nil || len(roots) == 0 {
		return dependency.NewGraph()
	}

	filter := dependency.Filter{
		NodeIDs:         roots,
		DependencyDepth: depths.Dependencies,
		DependentDepth:  depths.Dependents,
	}
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

// ClosureScope maps the flag-encoded closure depths (0 = off, -1 = unlimited,
// N>0 = N levels; see flags.ParseClosureDepth) onto a traversal Direction and
// per-direction Depths (filter encoding: 0 = unlimited). At least one of the
// two values must be non-zero; when both are set the direction is
// DirectionBoth with independent depths.
func ClosureScope(includeDependencies, includeDependents int) (Direction, Depths) {
	defer perf.Track(nil, "dependencies.ClosureScope")()

	depths := Depths{}
	if includeDependencies > 0 {
		depths.Dependencies = includeDependencies
	}
	if includeDependents > 0 {
		depths.Dependents = includeDependents
	}
	switch {
	case includeDependencies != 0 && includeDependents != 0:
		return DirectionBoth, depths
	case includeDependencies != 0:
		return DirectionForward, depths
	default:
		return DirectionReverse, depths
	}
}

// Membership returns the set of (component, stack) node IDs present in graph,
// keyed by NodeID. Row-shaped consumers (e.g. `list instances`) use it to keep
// exactly the rows a closure selects.
func Membership(graph *dependency.Graph) map[string]struct{} {
	defer perf.Track(nil, "dependencies.Membership")()

	if graph == nil {
		return map[string]struct{}{}
	}
	members := make(map[string]struct{}, len(graph.Nodes))
	for id := range graph.Nodes {
		members[id] = struct{}{}
	}
	return members
}

// ComponentNames returns the sorted, deduplicated component names present in
// graph. Used by `list components` to keep exactly the components a closure
// selects.
func ComponentNames(graph *dependency.Graph) []string {
	defer perf.Track(nil, "dependencies.ComponentNames")()

	if graph == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(graph.Nodes))
	names := make([]string, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		if _, ok := seen[node.Component]; ok {
			continue
		}
		seen[node.Component] = struct{}{}
		names = append(names, node.Component)
	}
	sort.Strings(names)
	return names
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
