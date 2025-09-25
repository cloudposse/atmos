package dependency

import (
	"fmt"
)

// Constants for common error formats.
const (
	errWithContextFormat = "%w: %s"
)

// NewGraph creates a new dependency graph.
func NewGraph() *Graph {
	return &Graph{
		Nodes: make(map[string]*Node),
		Roots: []string{},
	}
}

// AddNode adds a node to the graph.
func (g *Graph) AddNode(node *Node) error {
	if node == nil {
		return ErrNilNode
	}

	if node.ID == "" {
		return ErrEmptyNodeID
	}

	if _, exists := g.Nodes[node.ID]; exists {
		return fmt.Errorf(errWithContextFormat, ErrNodeExists, node.ID)
	}

	// Initialize slices if nil
	if node.Dependencies == nil {
		node.Dependencies = []string{}
	}
	if node.Dependents == nil {
		node.Dependents = []string{}
	}

	g.Nodes[node.ID] = node
	return nil
}

// AddDependency creates a dependency relationship between two nodes.
// The fromID depends on toID (fromID -> toID).
func (g *Graph) AddDependency(fromID, toID string) error {
	if fromID == "" || toID == "" {
		return ErrEmptyDependencyID
	}

	if fromID == toID {
		return fmt.Errorf(errWithContextFormat, ErrSelfDependency, fromID)
	}

	fromNode, fromExists := g.Nodes[fromID]
	if !fromExists {
		return fmt.Errorf(errWithContextFormat, ErrNodeNotFound, fromID)
	}

	toNode, toExists := g.Nodes[toID]
	if !toExists {
		return fmt.Errorf(errWithContextFormat, ErrNodeNotFound, toID)
	}

	// Check if dependency already exists
	for _, dep := range fromNode.Dependencies {
		if dep == toID {
			return nil // Dependency already exists, skip
		}
	}

	// Add the dependency relationship
	fromNode.Dependencies = append(fromNode.Dependencies, toID)
	toNode.Dependents = append(toNode.Dependents, fromID)

	return nil
}

// IdentifyRoots finds all nodes with no dependencies.
func (g *Graph) IdentifyRoots() {
	g.Roots = []string{}

	for id, node := range g.Nodes {
		if len(node.Dependencies) == 0 {
			g.Roots = append(g.Roots, id)
		}
	}
}

// GetNode retrieves a node by its ID.
func (g *Graph) GetNode(id string) (*Node, bool) {
	node, exists := g.Nodes[id]
	return node, exists
}

// Size returns the number of nodes in the graph.
func (g *Graph) Size() int {
	return len(g.Nodes)
}

// HasCycles checks if the graph contains any cycles.
func (g *Graph) HasCycles() (bool, []string) {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	var cyclePath []string

	// Helper function for DFS
	var dfs func(nodeID string) bool
	dfs = func(nodeID string) bool {
		visited[nodeID] = true
		recStack[nodeID] = true

		node := g.Nodes[nodeID]
		for _, depID := range node.Dependencies {
			if !visited[depID] {
				if dfs(depID) {
					cyclePath = append([]string{nodeID}, cyclePath...)
					return true
				}
			} else if recStack[depID] {
				// Cycle detected
				cyclePath = []string{nodeID, depID}
				return true
			}
		}

		recStack[nodeID] = false
		return false
	}

	// Check all unvisited nodes
	for id := range g.Nodes {
		if !visited[id] {
			if dfs(id) {
				return true, cyclePath
			}
		}
	}

	return false, nil
}

// Reset clears the processed flag for all nodes.
func (g *Graph) Reset() {
	for _, node := range g.Nodes {
		node.Processed = false
	}
}

// Clone creates a deep copy of the graph.
func (g *Graph) Clone() *Graph {
	newGraph := NewGraph()

	// Clone all nodes
	for id, node := range g.Nodes {
		newNode := &Node{
			ID:           node.ID,
			Component:    node.Component,
			Stack:        node.Stack,
			Type:         node.Type,
			Dependencies: make([]string, len(node.Dependencies)),
			Dependents:   make([]string, len(node.Dependents)),
			Metadata:     node.Metadata, // Note: This is a shallow copy of the map
			Processed:    node.Processed,
		}

		// Copy slices
		copy(newNode.Dependencies, node.Dependencies)
		copy(newNode.Dependents, node.Dependents)

		newGraph.Nodes[id] = newNode
	}

	// Copy roots
	newGraph.Roots = make([]string, len(g.Roots))
	copy(newGraph.Roots, g.Roots)

	return newGraph
}
