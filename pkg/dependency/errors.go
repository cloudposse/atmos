package dependency

import "errors"

// Static error definitions for the dependency package.
var (
	// ErrNilNode is returned when attempting to add a nil node to the graph.
	ErrNilNode = errors.New("cannot add nil node to graph")

	// ErrEmptyNodeID is returned when a node has an empty ID.
	ErrEmptyNodeID = errors.New("node ID cannot be empty")

	// ErrNodeExists is returned when attempting to add a node that already exists.
	ErrNodeExists = errors.New("node already exists in graph")

	// ErrEmptyDependencyID is returned when dependency IDs are empty.
	ErrEmptyDependencyID = errors.New("dependency IDs cannot be empty")

	// ErrSelfDependency is returned when a node attempts to depend on itself.
	ErrSelfDependency = errors.New("node cannot depend on itself")

	// ErrNodeNotFound is returned when a referenced node doesn't exist in the graph.
	ErrNodeNotFound = errors.New("node does not exist in graph")

	// ErrGraphAlreadyBuilt is returned when attempting to modify a built graph.
	ErrGraphAlreadyBuilt = errors.New("graph has already been built")

	// ErrCircularDependency is returned when a circular dependency is detected.
	ErrCircularDependency = errors.New("circular dependency detected")

	// ErrNoRootNodes is returned when no root nodes are found in the graph.
	ErrNoRootNodes = errors.New("no root nodes found - possible circular dependency involving all nodes")
)
