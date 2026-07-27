package dependencies

import (
	"path"
	"sort"

	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/perf"
)

// DescribeFunc runs one describe-stacks pass. An empty stack means every
// stack; processTemplates/processFunctions control template and YAML-function
// evaluation for the pass. Callers must NOT narrow the describe by tags,
// labels, or components — scoping is the closure's job, and a narrowed
// describe would drop dependency/dependent components from the graph before
// the closure could see them.
type DescribeFunc func(stack string, processTemplates, processFunctions bool) (map[string]any, error)

// ScopeRequest bounds a scoped closure resolution: the selection seed
// (components/stack/tags/labels), the closure direction and depths, and the
// evaluation settings for the resolved passes.
type ScopeRequest struct {
	// Components optionally narrows the seed to these component names.
	Components []string
	// Stack optionally narrows the seed to a single stack.
	Stack string
	// Tags narrows the seed to components whose metadata.tags match (any-match).
	Tags []string
	// Labels narrows the seed to components whose metadata.labels match (all-match).
	Labels map[string]string
	// Direction selects forward (dependencies), reverse (dependents), or both.
	Direction Direction
	// Depths bounds the closure expansion per direction (0 = unlimited).
	Depths Depths
	// ProcessTemplates/ProcessFunctions control evaluation in the resolved
	// (Phase C) describe passes; the lightweight pass always runs with both off.
	ProcessTemplates bool
	ProcessFunctions bool
	// LeftDelim is the configured left template delimiter ("{{" when empty),
	// used to conservatively match still-unresolved selectors in the
	// lightweight graph.
	LeftDelim string
}

// ScopeResult is the outcome of a scoped closure resolution.
type ScopeResult struct {
	// Stacks is the merged union of the resolved per-stack describe passes —
	// exactly the stacks the closure touches, fully evaluated. Feed this to
	// graph-backed execution (e.g. scheduleradapters.ExecuteTerraform).
	Stacks map[string]any
	// Closure is the final reachable closure computed against the resolved
	// graph. List commands use it to filter rows.
	Closure *dependency.Graph
}

// Selector bundles the seed-matching filters for root selection: components
// (empty = every component), a stack (exact name or path.Match glob — the
// `list` commands' --stack semantics), tags (any-match), labels (all-match),
// and the configured left template delimiter for detecting unresolved
// selectors. An empty filter matches every node, and a templated
// (unresolvable) tags/labels selector conservatively matches so a lightweight
// unevaluated graph can never wrongly exclude a root.
type Selector struct {
	Components []string
	Stack      string
	Tags       []string
	Labels     map[string]string
	LeftDelim  string
}

// Roots returns the sorted IDs of nodes matching the selector, for use as
// ReachableClosure roots.
func Roots(graph *dependency.Graph, sel *Selector) []string {
	defer perf.Track(nil, "dependencies.Roots")()

	if graph == nil || sel == nil {
		return nil
	}
	var ids []string
	for _, node := range graph.Nodes {
		if !sel.matches(node) {
			continue
		}
		ids = append(ids, node.ID)
	}
	sort.Strings(ids)
	return ids
}

// matches reports whether a node satisfies every selector filter.
func (s *Selector) matches(node *dependency.Node) bool {
	if len(s.Components) > 0 && !containsString(s.Components, node.Component) {
		return false
	}
	if !stackMatches(s.Stack, node.Stack) {
		return false
	}
	return nodeMatchesTagsLabels(node, s.Tags, s.Labels, s.LeftDelim)
}

// stackMatches reports whether a stack name satisfies the stack filter: an
// empty filter matches everything, an exact name matches itself, and a glob
// pattern is matched via path.Match (a malformed pattern simply falls back to
// the exact comparison above).
func stackMatches(pattern, stack string) bool {
	if pattern == "" || pattern == stack {
		return true
	}
	matched, err := path.Match(pattern, stack)
	return err == nil && matched
}

// ResolveScopedClosure implements the three-phase scoped evaluation shared by
// `atmos list dependencies` and the terraform bulk commands:
//
//   - Phase A: one lightweight structural describe across every stack
//     (templates/YAML functions off — component identity plus
//     dependencies.components/settings.depends_on edges, which are static in
//     practice; see the fix doc's grep of examples/tests).
//   - Phase B: seed roots from the request's selection filters, then compute
//     the reachable closure in the requested direction(s) and depth(s).
//   - Phase C: fully evaluate ONLY the stacks the closure touches, then
//     recompute the closure against the resolved graph. If the resolved edges
//     reveal additional stacks the lightweight pass could not see (e.g. a
//     templated `stack:` dependency target), describe those too and recompute
//     again — repeating until the touched-stack set stabilizes. This
//     terminates in at most len(repo stacks) rounds, since each round only
//     adds genuinely new, structurally-discovered stacks.
//
// Known limitation: a dependency target `stack:` value templated to point at
// a DIFFERENT stack converges once that stack is described (the loop expands
// and redescribes until stable), but only within the stacks reachable that
// way — it cannot discover a target stack whose name depends on a value this
// pass never touches (e.g. a remote store lookup). The
// describe.settings.eager_evaluation setting remains the fallback to the
// historical full-repo-evaluation behavior.
func ResolveScopedClosure(describe DescribeFunc, req *ScopeRequest) (*ScopeResult, error) {
	defer perf.Track(nil, "dependencies.ResolveScopedClosure")()

	lightweightStacks, err := describe("", false, false)
	if err != nil {
		return nil, err
	}
	graph, err := BuildGraph(lightweightStacks)
	if err != nil {
		return nil, err
	}
	if graph.Size() == 0 {
		return &ScopeResult{Stacks: lightweightStacks, Closure: graph}, nil
	}

	roots := Roots(graph, &Selector{
		Components: req.Components,
		Stack:      req.Stack,
		Tags:       req.Tags,
		Labels:     req.Labels,
		LeftDelim:  req.LeftDelim,
	})
	closure := ReachableClosure(graph, roots, req.Direction, req.Depths)
	if closure.Size() == 0 {
		return &ScopeResult{Stacks: map[string]any{}, Closure: closure}, nil
	}

	return resolveClosureStacks(describe, req, roots, StackNames(closure))
}

// resolveClosureStacks is Phase C: re-describe (templates/YAML functions/auth
// per the request) each stack in stackNames, then recompute the reachable
// closure against that resolved graph, expanding the stack set until stable.
// Reachability is always recomputed against the resolved graph (never reused
// from the lightweight pass), so a same-stack templated dependency target
// (e.g. `stack: "{{ .vars.stage }}"` resolving to its own stack) still
// produces the correct closure.
func resolveClosureStacks(describe DescribeFunc, req *ScopeRequest, roots, stackNames []string) (*ScopeResult, error) {
	resolvedStacks := make(map[string]any)
	described := make(map[string]bool)
	var resolvedGraph *dependency.Graph

	for {
		pending := pendingStackNames(stackNames, described)
		if len(pending) == 0 {
			return &ScopeResult{
				Stacks:  resolvedStacks,
				Closure: ReachableClosure(resolvedGraph, roots, req.Direction, req.Depths),
			}, nil
		}

		for _, stackName := range pending {
			partial, err := describe(stackName, req.ProcessTemplates, req.ProcessFunctions)
			if err != nil {
				return nil, err
			}
			for k, v := range partial {
				resolvedStacks[k] = v
			}
			described[stackName] = true
		}

		var err error
		resolvedGraph, err = BuildGraph(resolvedStacks)
		if err != nil {
			return nil, err
		}
		stackNames = StackNames(ReachableClosure(resolvedGraph, roots, req.Direction, req.Depths))
	}
}

// pendingStackNames returns the stack names not yet described.
func pendingStackNames(stackNames []string, described map[string]bool) []string {
	pending := make([]string, 0, len(stackNames))
	for _, stackName := range stackNames {
		if !described[stackName] {
			pending = append(pending, stackName)
		}
	}
	return pending
}

// containsString reports whether values contains value.
func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
