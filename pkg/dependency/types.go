package dependency

// Node represents a component in the dependency graph.
type Node struct {
	// ID is the unique identifier for the node (typically component-stack).
	ID string

	// Component is the name of the component.
	Component string

	// Stack is the stack name where this component is defined.
	Stack string

	// Type indicates the component type (e.g., "terraform", "helmfile").
	Type string

	// Dependencies contains IDs of nodes that this node depends on.
	Dependencies []string

	// Dependents contains IDs of nodes that depend on this node.
	Dependents []string

	// Metadata stores additional component-specific data.
	Metadata map[string]any

	// Processed indicates whether this node has been processed during traversal.
	Processed bool
}

// Graph represents a dependency graph of components.
type Graph struct {
	// Nodes maps node IDs to their corresponding Node structures.
	Nodes map[string]*Node

	// Roots contains IDs of nodes with no dependencies (entry points).
	Roots []string
}

// Builder defines the interface for constructing dependency graphs.
type Builder interface {
	// AddNode adds a node to the graph being built.
	AddNode(node *Node) error

	// AddDependency creates a dependency relationship between two nodes.
	AddDependency(fromID, toID string) error

	// Build finalizes the graph construction and returns the built graph.
	Build() (*Graph, error)
}

// ExecutionOrder represents a slice of nodes in dependency order.
type ExecutionOrder []Node

// Filter defines options for filtering a dependency graph.
type Filter struct {
	// NodeIDs specifies which nodes to include.
	NodeIDs []string

	// IncludeDependencies indicates whether to include all dependencies of filtered nodes.
	IncludeDependencies bool

	// IncludeDependents indicates whether to include all dependents of filtered nodes.
	IncludeDependents bool
}
