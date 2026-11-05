package dependency

import (
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

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

	// ErrAddNodeFailed is returned when the builder fails to add a node.
	ErrAddNodeFailed = errors.New("builder failed to add node")

	// ErrAddDependencyFailed is returned when the builder fails to add a dependency.
	ErrAddDependencyFailed = errors.New("builder failed to add dependency")
)

// Reasons a required dependency target cannot be used.
const (
	// TargetMissing means no concrete component with the target name exists, or the target is abstract.
	TargetMissing = "target_missing"
	// TargetDisabled means the target exists but is disabled.
	TargetDisabled = "target_disabled"
)

// DependencyTargetError reports a required dependency whose target is missing or unavailable. It
// carries the declaring component and the target so diagnostics can point at the declaration.
type DependencyTargetError struct {
	Component       string
	Stack           string
	TargetComponent string
	TargetStack     string
	Reason          string
}

// Error keeps the text of the sentinel it wraps followed by the identity of both ends.
func (e *DependencyTargetError) Error() string {
	defer perf.Track(nil, "dependency.DependencyTargetError.Error")()

	return fmt.Sprintf("%v: component=%s stack=%s target_component=%s target_stack=%s reason=%s", e.Unwrap(), e.Component, e.Stack, e.TargetComponent, e.TargetStack, e.Reason)
}

// Unwrap returns ErrDependencyTargetNotFound for a missing target and ErrDependencyTargetUnavailable otherwise.
func (e *DependencyTargetError) Unwrap() error {
	defer perf.Track(nil, "dependency.DependencyTargetError.Unwrap")()

	if e.Reason == TargetMissing {
		return errUtils.ErrDependencyTargetNotFound
	}
	return errUtils.ErrDependencyTargetUnavailable
}

// DependencyDeclarationError attributes a graph error, such as an unparsable or unresolved
// dependency declaration or a cycle, to the component whose declaration caused it.
type DependencyDeclarationError struct {
	Component string
	Stack     string
	Err       error
}

// Error returns the text of the wrapped error unchanged.
func (e *DependencyDeclarationError) Error() string {
	defer perf.Track(nil, "dependency.DependencyDeclarationError.Error")()

	return e.Err.Error()
}

// Unwrap returns the wrapped error.
func (e *DependencyDeclarationError) Unwrap() error {
	defer perf.Track(nil, "dependency.DependencyDeclarationError.Unwrap")()

	return e.Err
}

// CircularDependencyError reports the nodes of a dependency cycle.
type CircularDependencyError struct {
	Path []string
}

// Error keeps the text callers have always seen for a cycle.
func (e *CircularDependencyError) Error() string {
	defer perf.Track(nil, "dependency.CircularDependencyError.Error")()

	return fmt.Sprintf("%v: %v", ErrCircularDependency, e.Path)
}

// Unwrap returns ErrCircularDependency.
func (e *CircularDependencyError) Unwrap() error {
	defer perf.Track(nil, "dependency.CircularDependencyError.Unwrap")()

	return ErrCircularDependency
}
