package dependencies

import (
	"maps"
	"path"
	"path/filepath"
	"slices"
	"sort"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/perf"
)

// DescribeFunc runs one describe-stacks pass. An empty stack means every
// stack; a non-nil components list narrows evaluation to those component
// names within the stack (nil = every component); processTemplates/
// processFunctions control template and YAML-function evaluation for the
// pass. Callers must NOT narrow the describe by tags or labels — scoping is
// the closure's job, and a narrowed describe would drop dependency/dependent
// components from the graph before the closure could see them. The components
// list is supplied BY the closure engine (never by the caller's own selection)
// so that evaluating a closure stack does not evaluate unrelated components
// that merely share the stack.
type DescribeFunc func(stack string, components []string, processTemplates, processFunctions bool) (map[string]any, error)

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
	// LeftDelim/RightDelim are the configured template delimiters ("{{"/"}}"
	// when empty), used to detect — and best-effort resolve — templated
	// selectors in the lightweight graph.
	LeftDelim  string
	RightDelim string
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
	RightDelim string
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
	if len(s.Components) > 0 && !slices.Contains(s.Components, node.Component) {
		return false
	}
	if !stackMatches(s.Stack, node.Stack) {
		return false
	}
	return nodeMatchesTagsLabels(node, s.Tags, s.Labels, s.LeftDelim, s.RightDelim)
}

// stackMatches reports whether a stack name satisfies the stack filter: an
// empty filter matches everything, an exact name matches itself, and a glob
// pattern is matched via path.Match (a malformed pattern simply falls back to
// the exact comparison above). Both sides are slash-normalized so the seed
// glob behaves the same as the non-closure instances path on Windows (see
// matchStackPattern in pkg/list/list_instances.go).
func stackMatches(pattern, stack string) bool {
	if pattern == "" || pattern == stack {
		return true
	}
	matched, err := path.Match(filepath.ToSlash(pattern), filepath.ToSlash(stack))
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
//
// Reverse/both directions additionally evaluate every component whose
// dependency declarations were unresolved in the lightweight pass (see
// UnresolvedDependencySources): a dependent with a templated edge would
// otherwise never be evaluated (it is not yet IN the closure) and thus never
// discovered. This is a conservative extra-evaluation cost, never an
// extra-execution cost — membership is still decided by the closure computed
// against the resolved graph.
func ResolveScopedClosure(describe DescribeFunc, req *ScopeRequest) (*ScopeResult, error) {
	defer perf.Track(nil, "dependencies.ResolveScopedClosure")()

	lightweightStacks, err := describe("", nil, false, false)
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
		RightDelim: req.RightDelim,
	})
	closure := ReachableClosure(graph, roots, req.Direction, req.Depths)
	if closure.Size() == 0 {
		return &ScopeResult{Stacks: map[string]any{}, Closure: closure}, nil
	}

	return resolveClosureStacks(describe, req, roots, closure, lightweightStacks)
}

// resolveClosureStacks is Phase C: re-describe (templates/YAML functions/auth
// per the request) each stack in stackNames, then recompute the reachable
// closure against that resolved graph, expanding the stack set until stable.
// Reachability is always recomputed against the resolved graph (never reused
// from the lightweight pass), so a same-stack templated dependency target
// (e.g. `stack: "{{ .vars.stage }}"` resolving to its own stack) still
// produces the correct closure.
func resolveClosureStacks(describe DescribeFunc, req *ScopeRequest, roots []string, closure *dependency.Graph, lightweightStacks map[string]any) (*ScopeResult, error) {
	resolvedStacks := make(map[string]any)
	evaluatedComponents := make(map[string]map[string]bool)
	var resolvedGraph *dependency.Graph

	// Reverse/both directions conservatively evaluate components whose
	// dependency declarations were unresolved in the lightweight pass — see
	// UnresolvedDependencySources and the ResolveScopedClosure doc comment.
	extraEval := map[string][]string{}
	if req.Direction != DirectionForward {
		extraEval = UnresolvedDependencySources(lightweightStacks, req.LeftDelim)
	}

	for {
		stackNames, componentsByStack := evaluationTargets(closure, extraEval)
		pending := pendingComponentsByStack(stackNames, componentsByStack, evaluatedComponents)
		if len(pending) == 0 {
			finalRoots := refineRoots(resolvedGraph, roots, req)
			return &ScopeResult{
				Stacks:  resolvedStacks,
				Closure: ReachableClosure(resolvedGraph, finalRoots, req.Direction, req.Depths),
			}, nil
		}

		// Evaluate only the closure's own components within each stack:
		// unrelated components that merely share a stack file with a closure
		// member must not have their templates/YAML functions (and thus their
		// auth/backend requirements) evaluated. Stacks with nothing left to
		// evaluate this round (all closure components already evaluated) are
		// skipped.
		for _, stackName := range stackNames {
			components, ok := pending[stackName]
			if !ok {
				continue
			}
			partial, err := describe(stackName, components, req.ProcessTemplates, req.ProcessFunctions)
			if err != nil {
				return nil, err
			}
			resolvedStacks = mergeResolvedClosureStacks(resolvedStacks, partial)
			if evaluatedComponents[stackName] == nil {
				evaluatedComponents[stackName] = make(map[string]bool, len(components))
			}
			for _, component := range components {
				evaluatedComponents[stackName][component] = true
			}
		}

		var err error
		resolvedGraph, err = BuildGraph(mergeResolvedClosureStacks(lightweightStacks, resolvedStacks))
		if err != nil {
			return nil, err
		}
		closure = ReachableClosure(resolvedGraph, roots, req.Direction, req.Depths)
	}
}

// refineRoots re-tests the seed roots against the RESOLVED graph so the final
// closure matches what execution will actually select. The lightweight-pass
// roots are a conservative superset: a templated selector that could not be
// resolved matched conservatively. Every root is a closure member, so by the
// time Phase C converges its selector values are resolved and can be tested
// exactly. Only the ORIGINAL roots are re-tested — never a full rescan of the
// resolved graph, which still contains unevaluated (conservatively-matching)
// non-closure nodes that must not join the seed now.
func refineRoots(resolvedGraph *dependency.Graph, roots []string, req *ScopeRequest) []string {
	if resolvedGraph == nil {
		return roots
	}
	sel := &Selector{
		Components: req.Components,
		Stack:      req.Stack,
		Tags:       req.Tags,
		Labels:     req.Labels,
		LeftDelim:  req.LeftDelim,
		RightDelim: req.RightDelim,
	}
	refined := make([]string, 0, len(roots))
	for _, id := range roots {
		node, ok := resolvedGraph.GetNode(id)
		if ok && sel.matches(node) {
			refined = append(refined, id)
		}
	}
	return refined
}

// evaluationTargets returns the stacks and per-stack components Phase C must
// evaluate this round: the closure's own members plus the conservative extras
// (components whose dependency declarations were unresolved in the lightweight
// pass — see UnresolvedDependencySources). Extras are evaluated so their edges
// materialize, but only the recomputed closure decides membership.
func evaluationTargets(closure *dependency.Graph, extraEval map[string][]string) ([]string, map[string][]string) {
	byStack := closureComponentsByStack(closure)
	for stack, components := range extraEval {
		byStack[stack] = append(byStack[stack], components...)
	}
	stacks := make([]string, 0, len(byStack))
	for stack := range byStack {
		sort.Strings(byStack[stack])
		byStack[stack] = slices.Compact(byStack[stack])
		stacks = append(stacks, stack)
	}
	sort.Strings(stacks)
	return stacks, byStack
}

// mergeResolvedClosureStacks overlays evaluated closure components onto the
// lightweight graph. The latter retains nodes that have not yet been evaluated,
// allowing a newly-resolved cross-stack edge to discover its target stack.
func mergeResolvedClosureStacks(lightweightStacks, resolvedStacks map[string]any) map[string]any {
	mergedStacks := maps.Clone(lightweightStacks)
	for stackName, resolvedValue := range resolvedStacks {
		lightweightStack, lightweightOK := lightweightStacks[stackName].(map[string]any)
		resolvedStack, resolvedOK := resolvedValue.(map[string]any)
		if !lightweightOK || !resolvedOK {
			mergedStacks[stackName] = resolvedValue
			continue
		}

		lightweightComponents, lightweightOK := lightweightStack[cfg.ComponentsSectionName].(map[string]any)
		resolvedComponents, resolvedOK := resolvedStack[cfg.ComponentsSectionName].(map[string]any)
		if !lightweightOK || !resolvedOK {
			mergedStacks[stackName] = resolvedValue
			continue
		}

		mergedStack := maps.Clone(lightweightStack)
		mergedComponents := maps.Clone(lightweightComponents)
		for componentType, resolvedTypeValue := range resolvedComponents {
			lightweightType, lightweightOK := lightweightComponents[componentType].(map[string]any)
			resolvedType, resolvedOK := resolvedTypeValue.(map[string]any)
			if !lightweightOK || !resolvedOK {
				mergedComponents[componentType] = resolvedTypeValue
				continue
			}
			mergedType := maps.Clone(lightweightType)
			maps.Copy(mergedType, resolvedType)
			mergedComponents[componentType] = mergedType
		}
		mergedStack[cfg.ComponentsSectionName] = mergedComponents
		mergedStacks[stackName] = mergedStack
	}
	return mergedStacks
}

// closureComponentsByStack maps each stack in the closure to the sorted
// component names the closure holds there.
func closureComponentsByStack(closure *dependency.Graph) map[string][]string {
	if closure == nil {
		return map[string][]string{}
	}
	byStack := make(map[string][]string, len(closure.Nodes))
	for _, node := range closure.Nodes {
		byStack[node.Stack] = append(byStack[node.Stack], node.Component)
	}
	for stack := range byStack {
		sort.Strings(byStack[stack])
	}
	return byStack
}

// pendingComponentsByStack returns closure components not yet evaluated per stack.
func pendingComponentsByStack(stackNames []string, componentsByStack map[string][]string, evaluated map[string]map[string]bool) map[string][]string {
	pending := make(map[string][]string)
	for _, stackName := range stackNames {
		for _, component := range componentsByStack[stackName] {
			if !evaluated[stackName][component] {
				pending[stackName] = append(pending[stackName], component)
			}
		}
	}
	return pending
}
