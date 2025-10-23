package exec

import (
	"fmt"
	"runtime"
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// DependencyNode represents a single node in the dependency resolution chain.
type DependencyNode struct {
	Component    string
	Stack        string
	FunctionType string // "terraform.state", "terraform.output", "atmos.Component"
	FunctionCall string // Full function call for error reporting
}

// ResolutionContext tracks the call stack during YAML function resolution to detect circular dependencies.
type ResolutionContext struct {
	CallStack []DependencyNode
	Visited   map[string]bool // Map of "stack-component" to track visited nodes
}

// goroutineResolutionContexts maps goroutine IDs to their resolution contexts.
var goroutineResolutionContexts sync.Map

// NewResolutionContext creates a new resolution context for cycle detection.
func NewResolutionContext() *ResolutionContext {
	defer perf.Track(nil, "exec.NewResolutionContext")()

	return &ResolutionContext{
		CallStack: make([]DependencyNode, 0),
		Visited:   make(map[string]bool),
	}
}

const (
	// Buffer size for capturing goroutine stack traces.
	goroutineStackBufSize = 64
)

// getGoroutineID returns the current goroutine ID.
func getGoroutineID() string {
	buf := make([]byte, goroutineStackBufSize)
	n := runtime.Stack(buf, false)
	// Format: "goroutine 123 [running]:\n..."
	buf = buf[:n]
	// Extract the number after "goroutine ".
	idField := strings.Fields(string(buf))[1]
	return idField
}

// GetOrCreateResolutionContext gets or creates a resolution context for the current goroutine.
func GetOrCreateResolutionContext() *ResolutionContext {
	defer perf.Track(nil, "exec.GetOrCreateResolutionContext")()

	gid := getGoroutineID()

	if ctx, ok := goroutineResolutionContexts.Load(gid); ok {
		return ctx.(*ResolutionContext)
	}

	ctx := NewResolutionContext()
	goroutineResolutionContexts.Store(gid, ctx)
	return ctx
}

// ClearResolutionContext clears the resolution context for the current goroutine.
func ClearResolutionContext() {
	defer perf.Track(nil, "exec.ClearResolutionContext")()

	gid := getGoroutineID()
	goroutineResolutionContexts.Delete(gid)
}

// Push adds a node to the call stack and checks for circular dependencies.
func (ctx *ResolutionContext) Push(atmosConfig *schema.AtmosConfiguration, node DependencyNode) error {
	defer perf.Track(atmosConfig, "exec.ResolutionContext.Push")()

	key := fmt.Sprintf("%s-%s", node.Stack, node.Component)

	// Check if we've already visited this node
	if ctx.Visited[key] {
		return ctx.buildCircularDependencyError(node)
	}

	// Mark as visited and add to call stack
	ctx.Visited[key] = true
	ctx.CallStack = append(ctx.CallStack, node)

	return nil
}

// Pop removes the top node from the call stack.
func (ctx *ResolutionContext) Pop(atmosConfig *schema.AtmosConfiguration) {
	defer perf.Track(atmosConfig, "exec.ResolutionContext.Pop")()

	if len(ctx.CallStack) > 0 {
		lastIdx := len(ctx.CallStack) - 1
		node := ctx.CallStack[lastIdx]
		key := fmt.Sprintf("%s-%s", node.Stack, node.Component)

		// Remove from visited set
		delete(ctx.Visited, key)

		// Remove from call stack
		ctx.CallStack = ctx.CallStack[:lastIdx]
	}
}

// buildCircularDependencyError creates a detailed error message showing the dependency chain.
func (ctx *ResolutionContext) buildCircularDependencyError(newNode DependencyNode) error {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("%s\n\n", errUtils.ErrCircularDependency))
	builder.WriteString("Dependency chain:\n")

	// Show the full call stack
	for i, node := range ctx.CallStack {
		builder.WriteString(fmt.Sprintf("  %d. Component '%s' in stack '%s'\n",
			i+1, node.Component, node.Stack))
		builder.WriteString(fmt.Sprintf("     → %s\n", node.FunctionCall))
	}

	// Show where the cycle completes
	builder.WriteString(fmt.Sprintf("  %d. Component '%s' in stack '%s' (cycle detected)\n",
		len(ctx.CallStack)+1, newNode.Component, newNode.Stack))
	builder.WriteString(fmt.Sprintf("     → %s\n\n", newNode.FunctionCall))

	builder.WriteString("To fix this issue:\n")
	builder.WriteString("  - Review your component dependencies and break the circular reference\n")
	builder.WriteString("  - Consider using Terraform data sources or direct remote state instead\n")
	builder.WriteString("  - Ensure dependencies flow in one direction only\n")

	return fmt.Errorf("%w: %s", errUtils.ErrCircularDependency, builder.String())
}

// Clone creates a copy of the resolution context for use in concurrent operations.
func (ctx *ResolutionContext) Clone() *ResolutionContext {
	defer perf.Track(nil, "exec.ResolutionContext.Clone")()

	if ctx == nil {
		return nil
	}

	newCtx := &ResolutionContext{
		CallStack: make([]DependencyNode, len(ctx.CallStack)),
		Visited:   make(map[string]bool, len(ctx.Visited)),
	}

	copy(newCtx.CallStack, ctx.CallStack)
	for k, v := range ctx.Visited {
		newCtx.Visited[k] = v
	}

	return newCtx
}
