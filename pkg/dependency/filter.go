package dependency

// Filter creates a new graph containing only the specified nodes and their relationships.
func (g *Graph) Filter(filter Filter) *Graph {
	filtered := NewGraph()
	toInclude := g.collectNodesToInclude(filter)
	g.copyNodesToFilteredGraph(filtered, toInclude)
	filtered.IdentifyRoots()
	return filtered
}

// collectNodesToInclude determines which nodes should be included based on the filter.
func (g *Graph) collectNodesToInclude(filter Filter) map[string]bool {
	toInclude := make(map[string]bool)

	for _, id := range filter.NodeIDs {
		if _, exists := g.Nodes[id]; !exists {
			continue
		}

		toInclude[id] = true

		if filter.IncludeDependencies {
			g.markDependencies(id, toInclude)
		}

		if filter.IncludeDependents {
			g.markDependents(id, toInclude)
		}
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

// markDependencies recursively marks all dependencies of a node for inclusion.
func (g *Graph) markDependencies(nodeID string, toInclude map[string]bool) {
	node, exists := g.Nodes[nodeID]
	if !exists {
		return
	}

	for _, depID := range node.Dependencies {
		if !toInclude[depID] {
			toInclude[depID] = true
			g.markDependencies(depID, toInclude)
		}
	}
}

// markDependents recursively marks all dependents of a node for inclusion.
func (g *Graph) markDependents(nodeID string, toInclude map[string]bool) {
	node, exists := g.Nodes[nodeID]
	if !exists {
		return
	}

	for _, depID := range node.Dependents {
		if !toInclude[depID] {
			toInclude[depID] = true
			g.markDependents(depID, toInclude)
		}
	}
}

// GetConnectedComponents returns all connected components in the graph.
// Each connected component is a subgraph where all nodes are reachable from each other.
func (g *Graph) GetConnectedComponents() []*Graph {
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
