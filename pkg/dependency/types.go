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

	// OptionalDependencies identifies outgoing edges that were declared optional.
	OptionalDependencies map[string]bool

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

	// DependencyDepth bounds how many dependency levels IncludeDependencies
	// pulls in, measured from the nearest filtered node (0 = unlimited).
	DependencyDepth int

	// DependentDepth bounds how many dependent levels IncludeDependents
	// pulls in, measured from the nearest filtered node (0 = unlimited).
	DependentDepth int
}

// SelectionFilter describes a selector-aware graph filter. It separates the
// kept selection (which dependencies expand from) from the unfiltered selection
// (which dependents expand from) and lets the caller drop reached dependents
// that do not match additional selectors.
type SelectionFilter struct {
	// Seeds is the kept selection. Dependencies expand from here, unfiltered.
	Seeds []string

	// DependentSeeds is the unfiltered selection that dependents expand from.
	// A nil slice means Seeds. Entries that are not in Seeds are treated as
	// dropped intermediates: they are never included, but dependents reached
	// through them are still considered and ordering is contracted around them.
	DependentSeeds []string

	// IncludeDependencies pulls in dependencies of Seeds without applying KeepDependent.
	IncludeDependencies bool

	// DependencyDepth bounds how many dependency levels are pulled in (0 = unlimited).
	DependencyDepth int

	// IncludeDependents pulls in dependents of DependentSeeds that pass KeepDependent.
	IncludeDependents bool

	// DependentDepth bounds how many dependent levels are walked, counted on the
	// full graph including dropped intermediates (0 = unlimited).
	DependentDepth int

	// KeepDependent reports whether a reached dependent is included. A nil
	// function keeps every reached dependent.
	KeepDependent func(*Node) bool
}
