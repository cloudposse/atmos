package dependencies

import (
	"fmt"
	"sort"
	"strconv"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/tags"
)

const (
	// Metadata subsection keys read by the --tags/--labels top-node filters.
	tagsMetadataKey   = "tags"
	labelsMetadataKey = "labels"

	// FormatLevels renders each node's shortest graph distance from a selected root.
	FormatLevels = "levels"
)

// Direction selects which dependency edges to display.
type Direction string

const (
	// DirectionBoth shows both what a component depends on and what depends on it.
	DirectionBoth Direction = "both"
	// DirectionForward shows what a component depends on.
	DirectionForward Direction = "forward"
	// DirectionReverse shows what depends on a component (its dependents).
	DirectionReverse Direction = "reverse"
)

// Options configures dependency rendering.
type Options struct {
	// Format is the output format: tree (default), json, yaml, or levels.
	Format string
	// Direction selects forward, reverse, or both edge directions.
	Direction Direction
	// Component optionally filters the top-level entries to a single component.
	Component string
	// Stack optionally filters the top-level entries to a single stack.
	Stack string
	// Tags optionally filters the top-level entries to components whose
	// metadata.tags contains at least one of these tags (any-match).
	Tags []string
	// Labels optionally filters the top-level entries to components whose
	// metadata.labels contains every key=value pair (all-match).
	Labels map[string]string
	// LeftDelim/RightDelim are the configured template delimiters ("{{"/"}}"
	// when empty), used to detect — and best-effort resolve — still-unresolved
	// selector templates under custom 'templates.settings.delimiters'
	// configurations.
	LeftDelim  string
	RightDelim string
}

// Render produces the dependency output for the given graph and options.
func Render(graph *dependency.Graph, opts Options) (string, error) {
	defer perf.Track(nil, "dependencies.Render")()

	if err := normalizeDirection(&opts); err != nil {
		return "", err
	}

	sel := &Selector{Stack: opts.Stack, Tags: opts.Tags, Labels: opts.Labels, LeftDelim: opts.LeftDelim, RightDelim: opts.RightDelim}
	if opts.Component != "" {
		sel.Components = []string{opts.Component}
	}
	tops := selectTopNodes(graph, sel)

	switch opts.Format {
	case "", string(format.FormatTree):
		return renderTree(graph, tops, opts.Direction), nil
	case string(format.FormatJSON), string(format.FormatYAML):
		return renderStructured(graph, tops, opts)
	case FormatLevels:
		return renderLevels(graph, tops, opts.Direction), nil
	default:
		return "", fmt.Errorf("%w: %q (supported: tree, json, yaml, %s)", errUtils.ErrInvalidFormat, opts.Format, FormatLevels)
	}
}

// renderLevels lists selected components and their shortest graph distance from
// a selected root in the requested direction.
func renderLevels(graph *dependency.Graph, tops []*dependency.Node, direction Direction) string {
	levels := levelDistances(graph, tops, direction)
	nodes := sortedLevelNodes(graph, levels)
	rows := make([][]string, 0, len(nodes))
	for _, node := range nodes {
		rows = append(rows, []string{strconv.Itoa(levels[node.ID]), node.Stack, node.Component, node.Type})
	}
	return format.CreateStyledTable([]string{"Level", "Stack", "Component", "Type"}, rows)
}

// levelDistances BFS-computes each reachable node's shortest distance from any
// of the selected roots (which sit at level 0) in the requested direction.
func levelDistances(graph *dependency.Graph, tops []*dependency.Node, direction Direction) map[string]int {
	levels := make(map[string]int, len(tops))
	queue := make([]string, 0, len(tops))
	for _, node := range tops {
		if _, seen := levels[node.ID]; seen {
			continue
		}
		levels[node.ID] = 0
		queue = append(queue, node.ID)
	}

	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		node, exists := graph.GetNode(id)
		if !exists {
			continue
		}
		for _, nextID := range levelNeighbors(node, direction) {
			if _, seen := levels[nextID]; seen {
				continue
			}
			levels[nextID] = levels[id] + 1
			queue = append(queue, nextID)
		}
	}
	return levels
}

// sortedLevelNodes resolves the levelled node IDs and orders them by level,
// then stack, then component for stable, readable output.
func sortedLevelNodes(graph *dependency.Graph, levels map[string]int) []*dependency.Node {
	nodes := make([]*dependency.Node, 0, len(levels))
	for id := range levels {
		if node, exists := graph.GetNode(id); exists {
			nodes = append(nodes, node)
		}
	}
	sort.Slice(nodes, func(i, j int) bool {
		if levels[nodes[i].ID] != levels[nodes[j].ID] {
			return levels[nodes[i].ID] < levels[nodes[j].ID]
		}
		if nodes[i].Stack != nodes[j].Stack {
			return nodes[i].Stack < nodes[j].Stack
		}
		return nodes[i].Component < nodes[j].Component
	})
	return nodes
}

func levelNeighbors(node *dependency.Node, direction Direction) []string {
	switch direction {
	case DirectionForward:
		return node.Dependencies
	case DirectionReverse:
		return node.Dependents
	default:
		return append(append([]string{}, node.Dependencies...), node.Dependents...)
	}
}

// normalizeDirection defaults an empty direction to DirectionBoth and rejects
// any unrecognized value.
func normalizeDirection(opts *Options) error {
	switch opts.Direction {
	case "":
		opts.Direction = DirectionBoth
		return nil
	case DirectionBoth, DirectionForward, DirectionReverse:
		return nil
	default:
		return fmt.Errorf("%w: direction %q (supported: both, forward, reverse)", errUtils.ErrInvalidFlag, opts.Direction)
	}
}

// selectTopNodes returns the sorted set of nodes to display as top-level entries,
// honoring optional component, stack, tags, and labels filters. Tags/labels
// filtering scopes which components appear as top-level tree entries (like
// component/stack do); dependency subtrees still traverse the full graph.
func selectTopNodes(graph *dependency.Graph, sel *Selector) []*dependency.Node {
	var nodes []*dependency.Node
	for _, node := range graph.Nodes {
		if !sel.matches(node) {
			continue
		}
		nodes = append(nodes, node)
	}
	sortNodes(nodes)
	return nodes
}

// nodeMatchesTagsLabels reports whether the node's metadata.tags (any-match)
// and metadata.labels (all-match) satisfy the filters. Node.Metadata holds
// the entire raw component section (see BuildGraph), so the metadata
// subsection is unwrapped first. A templated selector value is best-effort
// resolved against the component's raw section data (the purity contract
// limits selectors to simple templates, so this usually succeeds even on a
// lightweight unevaluated graph); a value that still cannot be resolved
// conservatively counts as a match — an unresolved selector must never
// wrongly exclude a component. The final render/adapter pass sees fully
// resolved values and filters exactly.
func nodeMatchesTagsLabels(node *dependency.Node, tagsFilter []string, labelsFilter map[string]string, leftDelim, rightDelim string) bool {
	if len(tagsFilter) == 0 && len(labelsFilter) == 0 {
		return true
	}

	metadata, _ := node.Metadata[cfg.MetadataSectionName].(map[string]any)
	rawTags := metadata[tagsMetadataKey]
	if len(tagsFilter) > 0 && tags.SelectorUnresolved(rawTags, leftDelim) {
		resolvedTags, ok := tags.ResolveSelectorValue(rawTags, node.Metadata, leftDelim, rightDelim)
		if !ok {
			return true
		}
		rawTags = resolvedTags
	}

	rawLabels := any(tags.RequestedLabels(metadata[labelsMetadataKey], labelsFilter))
	if len(labelsFilter) > 0 && tags.SelectorUnresolved(rawLabels, leftDelim) {
		resolvedLabels, ok := tags.ResolveSelectorValue(rawLabels, node.Metadata, leftDelim, rightDelim)
		if !ok {
			return true
		}
		rawLabels = resolvedLabels
	}

	return tags.MatchesTags(tags.ToStringSlice(rawTags), tagsFilter, tags.TagModeAny) &&
		tags.MatchesLabels(tags.ToStringMap(rawLabels), labelsFilter)
}

// sortNodes orders nodes by stack then component for stable, readable output.
func sortNodes(nodes []*dependency.Node) {
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Stack != nodes[j].Stack {
			return nodes[i].Stack < nodes[j].Stack
		}
		return nodes[i].Component < nodes[j].Component
	})
}

// renderTree builds the tree-format output for the selected top nodes.
func renderTree(graph *dependency.Graph, tops []*dependency.Node, direction Direction) string {
	showBoth := direction == DirectionBoth
	entries := make([]*format.DepTreeEntry, 0, len(tops))
	for _, node := range tops {
		entry := &format.DepTreeEntry{
			Component: node.Component,
			Stack:     node.Stack,
			Type:      node.Type,
			ShowBoth:  showBoth,
		}
		if direction == DirectionForward || direction == DirectionBoth {
			entry.DependsOn = buildSubtree(graph, node, forward, map[string]bool{node.ID: true})
		}
		if direction == DirectionReverse || direction == DirectionBoth {
			entry.RequiredBy = buildSubtree(graph, node, reverse, map[string]bool{node.ID: true})
		}
		entries = append(entries, entry)
	}
	return format.RenderDependenciesTree(entries)
}

// edgeDirection selects which adjacency list to traverse.
type edgeDirection int

const (
	forward edgeDirection = iota
	reverse
)

// neighbors returns the dependency or dependent IDs for a node based on direction.
func neighbors(node *dependency.Node, dir edgeDirection) []string {
	if dir == forward {
		return node.Dependencies
	}
	return node.Dependents
}

// buildSubtree recursively builds the dependency subtree for a node in the given
// direction. The path set guards against infinite recursion on cycles: a node
// already on the current path is emitted as a circular reference and not
// expanded further.
func buildSubtree(graph *dependency.Graph, node *dependency.Node, dir edgeDirection, path map[string]bool) []*format.DepTreeNode {
	ids := neighbors(node, dir)
	if len(ids) == 0 {
		return nil
	}

	sorted := make([]string, len(ids))
	copy(sorted, ids)
	sort.Strings(sorted)

	children := make([]*format.DepTreeNode, 0, len(sorted))
	for _, id := range sorted {
		next, exists := graph.GetNode(id)
		if !exists {
			continue
		}
		child := &format.DepTreeNode{Component: next.Component, Stack: next.Stack, Type: next.Type}
		if path[id] {
			child.Circular = true
			children = append(children, child)
			continue
		}
		path[id] = true
		child.Children = buildSubtree(graph, next, dir, path)
		delete(path, id)
		children = append(children, child)
	}
	return children
}
