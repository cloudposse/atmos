package exec

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/function/resolution"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// DependencyNode represents a single node in the dependency resolution chain.
// This is a stack-specific wrapper around resolution.Node that adds component/stack semantics.
type DependencyNode struct {
	Component    string
	Stack        string
	FunctionType string // "terraform.state", "terraform.output", "atmos.Component".
	FunctionCall string // Full function call for error reporting.
}

// ResolutionContext tracks the call stack during YAML function resolution to detect circular dependencies.
// This wraps the generic resolution.Context with stack-specific behavior.
type ResolutionContext struct {
	inner *resolution.Context
}

// NewResolutionContext creates a new resolution context for cycle detection.
func NewResolutionContext() *ResolutionContext {
	defer perf.Track(nil, "exec.NewResolutionContext")()

	return &ResolutionContext{
		inner: resolution.NewContext(),
	}
}

// GetOrCreateResolutionContext gets or creates a resolution context for the current goroutine.
func GetOrCreateResolutionContext() *ResolutionContext {
	defer perf.Track(nil, "exec.GetOrCreateResolutionContext")()

	inner := resolution.GetOrCreateContext()
	return &ResolutionContext{inner: inner}
}

// ClearResolutionContext clears the resolution context for the current goroutine.
func ClearResolutionContext() {
	defer perf.Track(nil, "exec.ClearResolutionContext")()

	resolution.ClearContext()
}

// scopedResolutionContext creates a new scoped resolution context and returns a restore function.
// This prevents memory leaks and cross-call contamination by ensuring contexts are cleaned up.
// Usage:
//
//	restoreCtx := scopedResolutionContext()
//	defer restoreCtx()
func scopedResolutionContext() func() {
	return resolution.ScopedContext()
}

// toResolutionNode converts a DependencyNode to a resolution.Node.
func (node DependencyNode) toResolutionNode() resolution.Node {
	return resolution.Node{
		Key:      fmt.Sprintf("%s-%s", node.Stack, node.Component),
		Label:    fmt.Sprintf("Component '%s' in stack '%s'", node.Component, node.Stack),
		CallInfo: node.FunctionCall,
	}
}

// Push adds a node to the call stack and checks for circular dependencies.
func (ctx *ResolutionContext) Push(atmosConfig *schema.AtmosConfiguration, node DependencyNode) error {
	defer perf.Track(atmosConfig, "exec.ResolutionContext.Push")()

	err := ctx.inner.Push(node.toResolutionNode())
	if err != nil {
		// Convert the generic cycle error to our stack-specific error format.
		return ctx.buildCircularDependencyError(node)
	}
	return nil
}

// Pop removes the top node from the call stack.
func (ctx *ResolutionContext) Pop(atmosConfig *schema.AtmosConfiguration) {
	defer perf.Track(atmosConfig, "exec.ResolutionContext.Pop")()

	ctx.inner.Pop()
}

// buildCircularDependencyError creates a detailed error message showing the dependency chain.
// This preserves the existing error format for backward compatibility.
func (ctx *ResolutionContext) buildCircularDependencyError(newNode DependencyNode) error {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("%s\n\n", errUtils.ErrCircularDependency))
	builder.WriteString("Dependency chain:\n")

	// Show the full call stack - convert resolution.Nodes back to stack-specific format.
	for i, resNode := range ctx.inner.CallStack {
		// Parse the key to extract component and stack.
		component, stack := parseNodeKey(resNode.Key)
		builder.WriteString(fmt.Sprintf("  %d. Component '%s' in stack '%s'\n",
			i+1, component, stack))
		builder.WriteString(fmt.Sprintf("     → %s\n", resNode.CallInfo))
	}

	// Show where the cycle completes.
	builder.WriteString(fmt.Sprintf("  %d. Component '%s' in stack '%s' (cycle detected)\n",
		len(ctx.inner.CallStack)+1, newNode.Component, newNode.Stack))
	builder.WriteString(fmt.Sprintf("     → %s\n\n", newNode.FunctionCall))

	builder.WriteString("To fix this issue:\n")
	builder.WriteString("  - Review your component dependencies and break the circular reference\n")
	builder.WriteString("  - Consider using Terraform data sources or direct remote state instead\n")
	builder.WriteString("  - Ensure dependencies flow in one direction only\n")

	return fmt.Errorf("%w: %s", errUtils.ErrCircularDependency, builder.String())
}

// parseNodeKey extracts stack and component from a key in format "stack-component".
// KNOWN LIMITATION: This format uses a hyphen delimiter which can be ambiguous
// with hyphenated stack or component names (e.g., "my-stack-my-component" could be
// parsed as stack="my" component="stack-my-component" instead of the intended
// stack="my-stack" component="my-component"). This is acceptable because:
// 1. The key is only used for internal cycle detection, not user-facing output
// 2. The Label field in resolution.Node provides accurate component/stack names for display
// 3. Changing the delimiter would break backward compatibility
// If this becomes problematic, consider using a different delimiter like "::" or "\x00".
func parseNodeKey(key string) (component, stack string) {
	parts := strings.SplitN(key, "-", 2)
	if len(parts) == 2 {
		return parts[1], parts[0]
	}
	return key, ""
}

// Clone creates a copy of the resolution context for use in concurrent operations.
func (ctx *ResolutionContext) Clone() *ResolutionContext {
	defer perf.Track(nil, "exec.ResolutionContext.Clone")()

	if ctx == nil {
		return nil
	}

	return &ResolutionContext{
		inner: ctx.inner.Clone(),
	}
}

// CallStack returns the dependency nodes in the call stack.
// This provides backward compatibility for code that accesses CallStack directly.
func (ctx *ResolutionContext) CallStack() []DependencyNode {
	defer perf.Track(nil, "exec.ResolutionContext.CallStack")()

	if ctx == nil || ctx.inner == nil {
		return nil
	}

	nodes := make([]DependencyNode, len(ctx.inner.CallStack))
	for i, resNode := range ctx.inner.CallStack {
		component, stack := parseNodeKey(resNode.Key)
		nodes[i] = DependencyNode{
			Component:    component,
			Stack:        stack,
			FunctionType: "", // Not stored in resolution.Node.
			FunctionCall: resNode.CallInfo,
		}
	}
	return nodes
}

// Visited returns a map of visited nodes.
// This provides backward compatibility for code that accesses Visited directly.
func (ctx *ResolutionContext) Visited() map[string]bool {
	defer perf.Track(nil, "exec.ResolutionContext.Visited")()

	if ctx == nil || ctx.inner == nil {
		return nil
	}

	return ctx.inner.Visited
}
