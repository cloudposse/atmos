package dependency

import "fmt"

// GraphBuilder implements the Builder interface for constructing dependency graphs.
type GraphBuilder struct {
	graph *Graph
	// Track if build has been called to prevent modifications after build.
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
		return ErrGraphAlreadyBuilt
	}

	if err := b.graph.AddNode(node); err != nil {
		return fmt.Errorf("%w: id=%s: %v", ErrAddNodeFailed, node.ID, err)
	}
	return nil
}

// AddDependency creates a dependency relationship between two nodes.
// The fromID depends on toID (fromID -> toID).
func (b *GraphBuilder) AddDependency(fromID, toID string) error {
	if b.built {
		return ErrGraphAlreadyBuilt
	}

	if err := b.graph.AddDependency(fromID, toID); err != nil {
		return fmt.Errorf("%w: from=%s to=%s: %v", ErrAddDependencyFailed, fromID, toID, err)
	}
	return nil
}

// Build finalizes the graph construction and returns the built graph.
func (b *GraphBuilder) Build() (*Graph, error) {
	if b.built {
		return nil, ErrGraphAlreadyBuilt
	}

	// Validate the graph for cycles.
	if hasCycle, cyclePath := b.graph.HasCycles(); hasCycle {
		return nil, fmt.Errorf("%w: %v", ErrCircularDependency, cyclePath)
	}

	// Identify root nodes.
	b.graph.IdentifyRoots()

	// Check if we have at least one root node (unless the graph is empty).
	if len(b.graph.Nodes) > 0 && len(b.graph.Roots) == 0 {
		return nil, ErrNoRootNodes
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
