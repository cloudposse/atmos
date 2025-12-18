package resolution

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// DependencyNode represents a single node in the dependency resolution chain.
type DependencyNode struct {
	Component    string
	Stack        string
	FunctionType string // "terraform.state", "terraform.output", "atmos.Component".
	FunctionCall string // Full function call for error reporting.
}

// Context tracks the call stack during YAML function resolution to detect circular dependencies.
type Context struct {
	CallStack []DependencyNode
	Visited   map[string]bool // Map of "stack-component" to track visited nodes.
}

// goroutineContexts maps goroutine IDs to their resolution contexts.
var goroutineContexts sync.Map

// NewContext creates a new resolution context for cycle detection.
func NewContext() *Context {
	defer perf.Track(nil, "resolution.NewContext")()

	return &Context{
		CallStack: make([]DependencyNode, 0),
		Visited:   make(map[string]bool),
	}
}

const (
	// Initial buffer size for capturing goroutine stack traces.
	goroutineStackBufSize = 64
	// Maximum buffer size to prevent unbounded growth.
	maxGoroutineStackBufSize = 8192
)

// unknownIDCounter is used to generate unique fallback IDs when goroutine ID parsing fails.
var unknownIDCounter uint64

// getGoroutineID returns the current goroutine ID.
// Returns a unique "unknown-N" identifier if parsing fails to prevent panics
// and avoid metric collisions when multiple goroutines hit the fallback path.
func getGoroutineID() string {
	// Allocate buffer and grow it if needed to avoid truncation.
	buf := make([]byte, goroutineStackBufSize)
	for {
		n := runtime.Stack(buf, false)
		if n < len(buf) {
			// Buffer was large enough.
			buf = buf[:n]
			break
		}
		// Buffer was too small, double it and try again.
		if len(buf) >= maxGoroutineStackBufSize {
			// Safety limit reached, return unique fallback ID.
			return fmt.Sprintf("unknown-%d", atomic.AddUint64(&unknownIDCounter, 1))
		}
		buf = make([]byte, len(buf)*2)
	}

	// Format: "goroutine 123 [running]:\n..."
	// Parse defensively to avoid panics.
	fields := strings.Fields(string(buf))
	if len(fields) < 2 {
		return fmt.Sprintf("unknown-%d", atomic.AddUint64(&unknownIDCounter, 1))
	}

	// Extract the number after "goroutine ".
	return fields[1]
}

// GetOrCreate gets or creates a resolution context for the current goroutine.
func GetOrCreate() *Context {
	defer perf.Track(nil, "resolution.GetOrCreate")()

	gid := getGoroutineID()

	if ctx, ok := goroutineContexts.Load(gid); ok {
		return ctx.(*Context)
	}

	ctx := NewContext()
	goroutineContexts.Store(gid, ctx)
	return ctx
}

// Clear clears the resolution context for the current goroutine.
func Clear() {
	defer perf.Track(nil, "resolution.Clear")()

	gid := getGoroutineID()
	goroutineContexts.Delete(gid)
}

// Scoped creates a new scoped resolution context and returns a restore function.
// This prevents memory leaks and cross-call contamination by ensuring contexts are cleaned up.
// Usage:
//
//	restoreCtx := resolution.Scoped()
//	defer restoreCtx()
func Scoped() func() {
	defer perf.Track(nil, "resolution.Scoped")()

	gid := getGoroutineID()

	// Save the existing context (if any).
	var savedCtx *Context
	if ctx, ok := goroutineContexts.Load(gid); ok {
		savedCtx = ctx.(*Context)
	}

	// Install a fresh context.
	freshCtx := NewContext()
	goroutineContexts.Store(gid, freshCtx)

	// Return a restore function that reinstates the saved context or clears it.
	return func() {
		if savedCtx != nil {
			goroutineContexts.Store(gid, savedCtx)
		} else {
			goroutineContexts.Delete(gid)
		}
	}
}

// Push adds a node to the call stack and checks for circular dependencies.
func (ctx *Context) Push(atmosConfig *schema.AtmosConfiguration, node DependencyNode) error {
	defer perf.Track(atmosConfig, "resolution.Context.Push")()

	key := fmt.Sprintf("%s-%s", node.Stack, node.Component)

	// Check if we've already visited this node.
	if ctx.Visited[key] {
		return ctx.buildCircularDependencyError(node)
	}

	// Mark as visited and add to call stack.
	ctx.Visited[key] = true
	ctx.CallStack = append(ctx.CallStack, node)

	return nil
}

// Pop removes the top node from the call stack.
func (ctx *Context) Pop(atmosConfig *schema.AtmosConfiguration) {
	defer perf.Track(atmosConfig, "resolution.Context.Pop")()

	if len(ctx.CallStack) > 0 {
		lastIdx := len(ctx.CallStack) - 1
		node := ctx.CallStack[lastIdx]
		key := fmt.Sprintf("%s-%s", node.Stack, node.Component)

		// Remove from visited set.
		delete(ctx.Visited, key)

		// Remove from call stack.
		ctx.CallStack = ctx.CallStack[:lastIdx]
	}
}

// buildCircularDependencyError creates a detailed error message showing the dependency chain.
func (ctx *Context) buildCircularDependencyError(newNode DependencyNode) error {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("%s\n\n", errUtils.ErrCircularDependency))
	builder.WriteString("Dependency chain:\n")

	// Show the full call stack.
	for i, node := range ctx.CallStack {
		builder.WriteString(fmt.Sprintf("  %d. Component '%s' in stack '%s'\n",
			i+1, node.Component, node.Stack))
		builder.WriteString(fmt.Sprintf("     → %s\n", node.FunctionCall))
	}

	// Show where the cycle completes.
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
func (ctx *Context) Clone() *Context {
	defer perf.Track(nil, "resolution.Context.Clone")()

	if ctx == nil {
		return nil
	}

	newCtx := &Context{
		CallStack: make([]DependencyNode, len(ctx.CallStack)),
		Visited:   make(map[string]bool, len(ctx.Visited)),
	}

	copy(newCtx.CallStack, ctx.CallStack)
	for k, v := range ctx.Visited {
		newCtx.Visited[k] = v
	}

	return newCtx
}
