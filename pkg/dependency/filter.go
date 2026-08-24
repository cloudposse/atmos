package dependency

import "github.com/cloudposse/atmos/pkg/perf"

// Filter creates a new graph containing only the specified nodes and their relationships.
func (g *Graph) Filter(filter Filter) *Graph {
	defer perf.Track(nil, "dependency.Graph.Filter")()

	filtered := NewGraph()
	toInclude := g.collectNodesToInclude(filter)
	g.copyNodesToFilteredGraph(filtered, toInclude)
	filtered.IdentifyRoots()
	return filtered
}

// collectNodesToInclude determines which nodes should be included based on the filter.
func (g *Graph) collectNodesToInclude(filter Filter) map[string]bool {
	toInclude := make(map[string]bool)

	seeds := make([]string, 0, len(filter.NodeIDs))
	for _, id := range filter.NodeIDs {
		if _, exists := g.Nodes[id]; !exists {
			continue
		}
		toInclude[id] = true
		seeds = append(seeds, id)
	}

	if filter.IncludeDependencies {
		g.markReachable(seeds, toInclude, filter.DependencyDepth, dependencyEdges)
	}
	if filter.IncludeDependents {
		g.markReachable(seeds, toInclude, filter.DependentDepth, dependentEdges)
	}

	return toInclude
}

// copyNodesToFilteredGraph copies the included nodes to the filtered graph.
func (g *Graph) copyNodesToFilteredGraph(filtered *Graph, toInclude map[string]bool) {
	for id := range toInclude {
		node, exists := g.Nodes[id]
		if !exists {
			continue
		}

		newNode := g.cloneNodeForFilter(node, toInclude)
		filtered.Nodes[id] = newNode
	}
}

// cloneNodeForFilter creates a copy of a node with filtered relationships.
func (g *Graph) cloneNodeForFilter(node *Node, toInclude map[string]bool) *Node {
	return cloneNodeWithFilteredEdges(node, toInclude)
}

// FilterByType creates a new graph containing only nodes of the specified type.
func (g *Graph) FilterByType(nodeType string) *Graph {
	defer perf.Track(nil, "dependency.Graph.FilterByType")()

	nodeIDs := []string{}
	for id, node := range g.Nodes {
		if node.Type == nodeType {
			nodeIDs = append(nodeIDs, id)
		}
	}

	return g.Filter(Filter{
		NodeIDs:             nodeIDs,
		IncludeDependencies: true,
		IncludeDependents:   false,
	})
}

// FilterByStack creates a new graph containing only nodes from the specified stack.
func (g *Graph) FilterByStack(stack string) *Graph {
	defer perf.Track(nil, "dependency.Graph.FilterByStack")()

	nodeIDs := []string{}
	for id, node := range g.Nodes {
		if node.Stack == stack {
			nodeIDs = append(nodeIDs, id)
		}
	}

	return g.Filter(Filter{
		NodeIDs:             nodeIDs,
		IncludeDependencies: true,
		IncludeDependents:   false,
	})
}

// FilterByComponent creates a new graph containing only nodes with the specified component name.
func (g *Graph) FilterByComponent(component string) *Graph {
	defer perf.Track(nil, "dependency.Graph.FilterByComponent")()

	nodeIDs := []string{}
	for id, node := range g.Nodes {
		if node.Component == component {
			nodeIDs = append(nodeIDs, id)
		}
	}

	return g.Filter(Filter{
		NodeIDs:             nodeIDs,
		IncludeDependencies: true,
		IncludeDependents:   true,
	})
}

// dependencyEdges selects a node's dependency IDs for markReachable.
func dependencyEdges(node *Node) []string {
	return node.Dependencies
}

// dependentEdges selects a node's dependent IDs for markReachable.
func dependentEdges(node *Node) []string {
	return node.Dependents
}

// reachEntry is one BFS frontier item in markReachable.
type reachEntry struct {
	id    string
	depth int
}

// markReachable marks every node reachable from the seed set within maxDepth
// hops (0 = unlimited) following the given edge selector. All seeds enter the
// BFS at depth 0, so the first visit to a node is at its minimum depth from
// the nearest seed; the visited map doubles as cycle protection.
func (g *Graph) markReachable(seeds []string, toInclude map[string]bool, maxDepth int, edges func(*Node) []string) {
	visited := make(map[string]struct{}, len(seeds))
	queue := make([]reachEntry, 0, len(seeds))
	for _, id := range seeds {
		visited[id] = struct{}{}
		queue = append(queue, reachEntry{id: id, depth: 0})
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if maxDepth > 0 && current.depth >= maxDepth {
			continue
		}
		node, exists := g.Nodes[current.id]
		if !exists {
			continue
		}
		for _, nextID := range edges(node) {
			if _, seen := visited[nextID]; seen {
				continue
			}
			visited[nextID] = struct{}{}
			toInclude[nextID] = true
			queue = append(queue, reachEntry{id: nextID, depth: current.depth + 1})
		}
	}
}

// GetConnectedComponents returns all connected components in the graph.
// Each connected component is a subgraph where all nodes are reachable from each other.
func (g *Graph) GetConnectedComponents() []*Graph {
	defer perf.Track(nil, "dependency.Graph.GetConnectedComponents")()

	visited := make(map[string]bool)
	components := []*Graph{}

	// Find all connected components.
	for id := range g.Nodes {
		if !visited[id] {
			// Find all nodes in this component.
			componentNodeIDs := g.findConnectedComponent(id, visited)

			// Build the component graph.
			component := g.buildComponentGraph(componentNodeIDs)
			components = append(components, component)
		}
	}

	return components
}

// findConnectedComponent performs DFS to find all nodes connected to the start node.
// It marks all found nodes as visited and returns their IDs.
func (g *Graph) findConnectedComponent(startID string, visited map[string]bool) map[string]bool {
	componentNodeIDs := make(map[string]bool)
	g.traverseConnectedNodes(startID, visited, componentNodeIDs)
	return componentNodeIDs
}

// traverseConnectedNodes recursively visits all connected nodes using DFS.
func (g *Graph) traverseConnectedNodes(nodeID string, visited map[string]bool, componentNodeIDs map[string]bool) {
	if visited[nodeID] {
		return
	}

	visited[nodeID] = true
	componentNodeIDs[nodeID] = true

	node, exists := g.Nodes[nodeID]
	if !exists {
		return
	}

	// Visit all dependencies.
	for _, depID := range node.Dependencies {
		g.traverseConnectedNodes(depID, visited, componentNodeIDs)
	}

	// Visit all dependents.
	for _, depID := range node.Dependents {
		g.traverseConnectedNodes(depID, visited, componentNodeIDs)
	}
}

// buildComponentGraph creates a new graph containing only the specified nodes.
// It clones nodes and filters their edges to only include nodes within the component.
func (g *Graph) buildComponentGraph(componentNodeIDs map[string]bool) *Graph {
	component := NewGraph()

	// Clone each node with filtered edges.
	for nodeID := range componentNodeIDs {
		node, exists := g.Nodes[nodeID]
		if !exists {
			continue
		}

		// Clone the node with edges filtered to component nodes only.
		clonedNode := cloneNodeWithFilteredEdges(node, componentNodeIDs)
		component.Nodes[nodeID] = clonedNode
	}

	component.IdentifyRoots()
	return component
}

// RemoveNode removes a node and all its relationships from the graph.
func (g *Graph) RemoveNode(nodeID string) error {
	defer perf.Track(nil, "dependency.Graph.RemoveNode")()

	node, exists := g.Nodes[nodeID]
	if !exists {
		return nil // Node doesn't exist, nothing to remove.
	}

	g.removeNodeFromDependencies(nodeID, node)
	g.removeNodeFromDependents(nodeID, node)

	delete(g.Nodes, nodeID)
	g.IdentifyRoots()

	return nil
}

// removeNodeFromDependencies removes the node from its dependencies' dependents lists.
func (g *Graph) removeNodeFromDependencies(nodeID string, node *Node) {
	for _, depID := range node.Dependencies {
		depNode, exists := g.Nodes[depID]
		if !exists {
			continue
		}
		depNode.Dependents = removeStringFromSlice(depNode.Dependents, nodeID)
	}
}

// removeNodeFromDependents removes the node from its dependents' dependencies lists.
func (g *Graph) removeNodeFromDependents(nodeID string, node *Node) {
	for _, depID := range node.Dependents {
		depNode, exists := g.Nodes[depID]
		if !exists {
			continue
		}
		depNode.Dependencies = removeStringFromSlice(depNode.Dependencies, nodeID)
	}
}

// removeStringFromSlice removes a specific string from a slice.
func removeStringFromSlice(slice []string, toRemove string) []string {
	result := []string{}
	for _, item := range slice {
		if item != toRemove {
			result = append(result, item)
		}
	}
	return result
}
