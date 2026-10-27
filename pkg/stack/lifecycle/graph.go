// Package lifecycle plans and executes component operations across stack kinds.
package lifecycle

import (
	"errors"
	"fmt"
	"slices"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/tags"
)

// Selection selects seeds before expanding dependencies in the original graph.
type Selection struct {
	Stack string
	// A nil Affected selects all nodes; a non-nil empty slice selects none.
	Affected []schema.Affected
	// Zero disables expansion, -1 expands without a depth limit.
	Dependencies int
	Dependents   int
	Reverse      bool
}

// Notice explains a component excluded from execution.
type Notice struct {
	Kind, Stack, Component, Reason string
}

// Plan is an immutable scheduling graph with resolved command arguments.
type Plan struct {
	Graph    *dependency.Graph
	Commands map[string][]string
	Notices  []Notice
	Handlers map[string]*Handler
}

// NodeID identifies a component without collisions across kinds or stack names.
func NodeID(kind, stack, name string) string {
	defer perf.Track(nil, "lifecycle.NodeID")()

	return fmt.Sprintf("%d:%s|%d:%s|%d:%s", len(kind), kind, len(stack), stack, len(name), name)
}

// Label returns a readable fully qualified component identity.
func Label(n *dependency.Node) string {
	defer perf.Track(nil, "lifecycle.Label")()

	return n.Type + "/" + n.Stack + "/" + n.Component
}

// BuildPlan resolves selection, capability, and ordering before any operation runs.
func BuildPlan(stacks map[string]any, commands map[string]*Handler, selection Selection, leftDelim string) (*Plan, error) {
	defer perf.Track(nil, "lifecycle.BuildPlan")()

	g, problems := discoverGraph(stacks, leftDelim)
	p := &Plan{Commands: make(map[string][]string), Handlers: commands}
	seeds := selectSeeds(g, commands, selection, p)
	selected, err := expandSelection(g, commands, seeds, selection, p)
	if err != nil {
		return nil, err
	}
	// Validate reachable sources before contraction, including unchanged intermediates.
	reachable := g.Filter(dependency.Filter{NodeIDs: keys(selected), IncludeDependencies: true})
	for _, id := range keys(reachable.Nodes) {
		if err := problems[id]; err != nil {
			return nil, err
		}
	}
	if _, err := reachable.TopologicalSort(); err != nil {
		return nil, err
	}
	p.Graph, err = contractGraph(g, selected, selection.Reverse)
	if err != nil {
		return nil, err
	}
	for _, id := range keys(p.Graph.Nodes) {
		n := p.Graph.Nodes[id]
		metadata, _ := n.Metadata["metadata"].(map[string]any)
		if locked, _ := metadata["locked"].(bool); locked {
			return nil, fmt.Errorf("%w: %s is locked", errUtils.ErrInvalidConfig, Label(n))
		}
		args, err := commands[n.Type].Arguments(n)
		if err != nil {
			return nil, err
		}
		p.Commands[id] = args
	}
	sort.Slice(p.Notices, func(i, j int) bool {
		a, b := p.Notices[i], p.Notices[j]
		return NodeID(a.Kind, a.Stack, a.Component)+a.Reason < NodeID(b.Kind, b.Stack, b.Component)+b.Reason
	})
	return p, nil
}

func discoverGraph(stacks map[string]any, leftDelim string) (*dependency.Graph, map[string]error) {
	g := dependency.NewGraph()
	problems := make(map[string]error)
	for _, stack := range keys(stacks) {
		section, _ := stacks[stack].(map[string]any)
		addStackNodes(g, stack, section, problems)
	}
	for _, id := range keys(g.Nodes) {
		n := g.Nodes[id]
		deps, modern, err := component.Dependencies(n.Metadata, n.Type, n.Stack, leftDelim)
		if err != nil {
			problems[id] = fmt.Errorf("%s: %w", Label(n), err)
			continue
		}
		for i := range deps {
			if err := addEdge(g, n, &deps[i], modern, leftDelim); err != nil {
				problems[id] = errors.Join(problems[id], err)
			}
		}
	}
	g.IdentifyRoots()
	return g, problems
}

func addStackNodes(g *dependency.Graph, stack string, section map[string]any, problems map[string]error) {
	kinds, _ := section["components"].(map[string]any)
	for _, kind := range keys(kinds) {
		components, _ := kinds[kind].(map[string]any)
		for _, name := range keys(components) {
			config, _ := components[name].(map[string]any)
			if config == nil || unavailable(config) {
				continue
			}
			n := &dependency.Node{ID: NodeID(kind, stack, name), Type: kind, Stack: stack, Component: name, Metadata: config}
			if err := g.AddNode(n); err != nil {
				problems[n.ID] = err
			}
		}
	}
}

func unavailable(section map[string]any) bool {
	m, _ := section["metadata"].(map[string]any)
	enabled, specified := m["enabled"].(bool)
	return m["type"] == "abstract" || (specified && !enabled)
}

func addEdge(g *dependency.Graph, n *dependency.Node, d *schema.ComponentDependency, modern bool, leftDelim string) error {
	if !d.IsComponentDependency() {
		return nil
	}
	if d.Kind == "" {
		d.Kind = n.Type
	}
	if d.Stack == "" {
		d.Stack = n.Stack
	}
	if tags.SelectorUnresolved(d.Component, leftDelim) || tags.SelectorUnresolved(d.Stack, leftDelim) || tags.SelectorUnresolved(d.Kind, leftDelim) {
		return fmt.Errorf("%w: unresolved dependency on %s", errUtils.ErrDependencyResolution, Label(n))
	}
	id := NodeID(d.Kind, d.Stack, d.Component)
	if g.Nodes[id] == nil {
		return missingTarget(n, d, modern)
	}
	return g.AddDependencyWithOptional(n.ID, id, modern && !d.IsRequired())
}

func missingTarget(n *dependency.Node, d *schema.ComponentDependency, modern bool) error {
	if modern && d.IsRequired() {
		return fmt.Errorf("%w: %s requires %s/%s/%s", errUtils.ErrDependencyTargetNotFound, Label(n), d.Kind, d.Stack, d.Component)
	}
	return nil
}

func selectSeeds(g *dependency.Graph, commands map[string]*Handler, s Selection, p *Plan) []string {
	eligible := affectedEligibility(s, p)
	var seeds []string
	for _, id := range keys(g.Nodes) {
		n := g.Nodes[id]
		if s.Stack != "" && s.Stack != n.Stack {
			continue
		}
		if s.Affected != nil && !eligible[id] {
			continue
		}
		if _, ok := commands[n.Type]; !ok {
			p.exclude(n, "component kind does not define this operation")
			continue
		}
		seeds = append(seeds, id)
	}
	return seeds
}

func affectedEligibility(s Selection, p *Plan) map[string]bool {
	eligible := make(map[string]bool)
	for i := range s.Affected {
		a := &s.Affected[i]
		if s.Stack != "" && s.Stack != a.Stack {
			continue
		}
		if a.Deleted {
			p.Notices = append(p.Notices, Notice{a.ComponentType, a.Stack, a.Component, "deleted definition; infrastructure teardown requires --allow-destroy and deleted-definition recovery"})
			continue
		}
		eligible[NodeID(a.ComponentType, a.Stack, a.Component)] = true
	}
	return eligible
}

func (p *Plan) exclude(n *dependency.Node, reason string) {
	p.Notices = append(p.Notices, Notice{n.Type, n.Stack, n.Component, reason})
}

func expandSelection(g *dependency.Graph, commands map[string]*Handler, seeds []string, s Selection, p *Plan) (map[string]bool, error) {
	selected := make(map[string]bool)
	for _, id := range seeds {
		selected[id] = true
	}
	for _, direction := range []struct {
		depth        int
		dependencies bool
	}{{s.Dependencies, true}, {s.Dependents, false}} {
		if direction.depth == 0 {
			continue
		}
		walker := expansion{graph: g, handlers: commands, selected: selected, plan: p, upstream: direction.dependencies}
		if err := walker.run(seeds, direction.depth); err != nil {
			return nil, err
		}
	}
	return selected, nil
}

type expansion struct {
	graph    *dependency.Graph
	handlers map[string]*Handler
	selected map[string]bool
	plan     *Plan
	upstream bool
}

func (w *expansion) run(seeds []string, limit int) error {
	frontier := slices.Clone(seeds)
	seen := make(map[string]bool)
	for depth := 0; len(frontier) > 0 && (limit < 0 || depth < limit); depth++ {
		var next []string
		for _, id := range frontier {
			if seen[id] {
				continue
			}
			seen[id] = true
			targets, err := w.step(id)
			if err != nil {
				return err
			}
			next = append(next, targets...)
		}
		frontier = next
	}
	return nil
}

func (w *expansion) step(id string) ([]string, error) {
	n := w.graph.Nodes[id]
	edges := n.Dependents
	if w.upstream {
		edges = n.Dependencies
	}
	var next []string
	for _, target := range edges {
		supported := w.handlers[w.graph.Nodes[target].Type] != nil
		if !supported && w.upstream && !n.OptionalDependencies[target] {
			return nil, fmt.Errorf("%w: %s requires operation on %s", errUtils.ErrUnsupportedComponentType, Label(n), Label(w.graph.Nodes[target]))
		}
		if supported {
			w.selected[target] = true
		} else {
			w.plan.exclude(w.graph.Nodes[target], "expanded component does not define this operation")
		}
		if supported || !w.upstream {
			next = append(next, target)
		}
	}
	return next, nil
}

// contractGraph retains transitive ordering even when unchanged intermediate
// components are not selected. Reversal applies to edges, not just display order.
func contractGraph(g *dependency.Graph, selected map[string]bool, reverse bool) (*dependency.Graph, error) {
	// Supply every unselected node as a dropped intermediate to the existing
	// selector-aware contraction algorithm. Keep only the finalized selection.
	result := g.FilterSelection(&dependency.SelectionFilter{
		Seeds: keys(selected), DependentSeeds: keys(g.Nodes), IncludeDependents: true,
		KeepDependent: func(n *dependency.Node) bool { return selected[n.ID] },
	})
	if reverse {
		result = reverseGraph(result)
	}
	_, err := result.TopologicalSort()
	return result, err
}

func reverseGraph(g *dependency.Graph) *dependency.Graph {
	result := g.Filter(dependency.Filter{NodeIDs: keys(g.Nodes)})
	for id, n := range result.Nodes {
		n.Dependencies, n.Dependents = n.Dependents, n.Dependencies
		n.OptionalDependencies = make(map[string]bool)
		for _, parent := range n.Dependencies {
			n.OptionalDependencies[parent] = g.Nodes[parent].OptionalDependencies[id]
		}
	}
	result.IdentifyRoots()
	return result
}

func keys[V any](m map[string]V) []string {
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}
