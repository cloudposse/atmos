package dependencies

import (
	"fmt"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/perf"
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
	// Format is the output format: tree (default), json, or yaml.
	Format string
	// Direction selects forward, reverse, or both edge directions.
	Direction Direction
	// Component optionally filters the top-level entries to a single component.
	Component string
	// Stack optionally filters the top-level entries to a single stack.
	Stack string
}

// Render produces the dependency output for the given graph and options.
func Render(graph *dependency.Graph, opts Options) (string, error) {
	defer perf.Track(nil, "dependencies.Render")()

	if err := normalizeDirection(&opts); err != nil {
		return "", err
	}

	tops := selectTopNodes(graph, opts.Component, opts.Stack)

	switch opts.Format {
	case "", string(format.FormatTree):
		return renderTree(graph, tops, opts.Direction), nil
	case string(format.FormatJSON), string(format.FormatYAML):
		return renderStructured(graph, tops, opts)
	default:
		return "", fmt.Errorf("%w: %q (supported: tree, json, yaml)", errUtils.ErrInvalidFormat, opts.Format)
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
// honoring optional component and stack filters.
func selectTopNodes(graph *dependency.Graph, component, stack string) []*dependency.Node {
	var nodes []*dependency.Node
	for _, node := range graph.Nodes {
		if component != "" && node.Component != component {
			continue
		}
		if stack != "" && node.Stack != stack {
			continue
		}
		nodes = append(nodes, node)
	}
	sortNodes(nodes)
	return nodes
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
		child := &format.DepTreeNode{Component: next.Component, Stack: next.Stack}
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
