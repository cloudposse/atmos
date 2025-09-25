package dependency

import (
	"fmt"
)

// GraphBuilder implements the Builder interface for constructing dependency graphs.
type GraphBuilder struct {
	graph *Graph
	// Track if build has been called to prevent modifications after build
	built bool
}

// NewBuilder creates a new graph builder.
func NewBuilder() *GraphBuilder {
	return &GraphBuilder{
		graph: NewGraph(),
		built: false,
	}
}

// AddNode adds a node to the graph being built.
func (b *GraphBuilder) AddNode(node *Node) error {
	if b.built {
		return fmt.Errorf("cannot add node after graph has been built")
	}

	return b.graph.AddNode(node)
}

// AddDependency creates a dependency relationship between two nodes.
// fromID depends on toID (fromID -> toID).
func (b *GraphBuilder) AddDependency(fromID, toID string) error {
	if b.built {
		return fmt.Errorf("cannot add dependency after graph has been built")
	}

	return b.graph.AddDependency(fromID, toID)
}

// Build finalizes the graph construction and returns the built graph.
func (b *GraphBuilder) Build() (*Graph, error) {
	if b.built {
		return nil, fmt.Errorf("graph has already been built")
	}

	// Validate the graph for cycles
	if hasCycle, cyclePath := b.graph.HasCycles(); hasCycle {
		return nil, fmt.Errorf("circular dependency detected: %v", cyclePath)
	}

	// Identify root nodes
	b.graph.IdentifyRoots()

	// Check if we have at least one root node (unless the graph is empty)
	if len(b.graph.Nodes) > 0 && len(b.graph.Roots) == 0 {
		return nil, fmt.Errorf("no root nodes found - possible circular dependency involving all nodes")
	}

	b.built = true
	return b.graph, nil
}

// GetGraph returns the current state of the graph (for debugging purposes).
// Note: This should not be used in production code; use Build() instead.
func (b *GraphBuilder) GetGraph() *Graph {
	return b.graph
}

// Reset resets the builder to start building a new graph.
func (b *GraphBuilder) Reset() {
	b.graph = NewGraph()
	b.built = false
}
