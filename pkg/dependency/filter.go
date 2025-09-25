package dependency

// Filter creates a new graph containing only the specified nodes and their relationships.
func (g *Graph) Filter(filter Filter) *Graph {
	filtered := NewGraph()
	toInclude := make(map[string]bool)

	// Mark nodes to include based on filter
	for _, id := range filter.NodeIDs {
		if _, exists := g.Nodes[id]; exists {
			toInclude[id] = true

			// Include dependencies if requested
			if filter.IncludeDependencies {
				g.markDependencies(id, toInclude)
			}

			// Include dependents if requested
			if filter.IncludeDependents {
				g.markDependents(id, toInclude)
			}
		}
	}

	// Copy included nodes to the filtered graph
	for id := range toInclude {
		if node, exists := g.Nodes[id]; exists {
			// Create a new node with the same data
			newNode := &Node{
				ID:           node.ID,
				Component:    node.Component,
				Stack:        node.Stack,
				Type:         node.Type,
				Dependencies: []string{},
				Dependents:   []string{},
				Metadata:     node.Metadata,
				Processed:    node.Processed,
			}

			// Only include dependencies/dependents that are also in the filtered set
			for _, depID := range node.Dependencies {
				if toInclude[depID] {
					newNode.Dependencies = append(newNode.Dependencies, depID)
				}
			}

			for _, depID := range node.Dependents {
				if toInclude[depID] {
					newNode.Dependents = append(newNode.Dependents, depID)
				}
			}

			filtered.Nodes[id] = newNode
		}
	}

	// Identify roots in the filtered graph
	filtered.IdentifyRoots()

	return filtered
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

	// DFS to find all nodes in a connected component
	var dfs func(nodeID string, component *Graph)
	dfs = func(nodeID string, component *Graph) {
		if visited[nodeID] {
			return
		}
		visited[nodeID] = true

		node := g.Nodes[nodeID]
		component.Nodes[nodeID] = node

		// Visit all connected nodes (both dependencies and dependents)
		for _, depID := range node.Dependencies {
			dfs(depID, component)
		}
		for _, depID := range node.Dependents {
			dfs(depID, component)
		}
	}

	// Find all connected components
	for id := range g.Nodes {
		if !visited[id] {
			component := NewGraph()
			dfs(id, component)
			component.IdentifyRoots()
			components = append(components, component)
		}
	}

	return components
}

// RemoveNode removes a node and all its relationships from the graph.
func (g *Graph) RemoveNode(nodeID string) error {
	node, exists := g.Nodes[nodeID]
	if !exists {
		return nil // Node doesn't exist, nothing to remove
	}

	// Remove this node from its dependencies' dependents lists
	for _, depID := range node.Dependencies {
		if depNode, exists := g.Nodes[depID]; exists {
			newDependents := []string{}
			for _, dependent := range depNode.Dependents {
				if dependent != nodeID {
					newDependents = append(newDependents, dependent)
				}
			}
			depNode.Dependents = newDependents
		}
	}

	// Remove this node from its dependents' dependencies lists
	for _, depID := range node.Dependents {
		if depNode, exists := g.Nodes[depID]; exists {
			newDependencies := []string{}
			for _, dependency := range depNode.Dependencies {
				if dependency != nodeID {
					newDependencies = append(newDependencies, dependency)
				}
			}
			depNode.Dependencies = newDependencies
		}
	}

	// Remove the node from the graph
	delete(g.Nodes, nodeID)

	// Update roots
	g.IdentifyRoots()

	return nil
}
