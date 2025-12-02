package resolution

import (
	"fmt"
	"runtime"
	"strings"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Node represents a single node in a dependency resolution chain.
// This is a generalized version that can be used for any cycle detection scenario.
type Node struct {
	// Key is the unique identifier for this node (e.g., "stack-component").
	Key string

	// Label is a human-readable description for error messages
	// (e.g., "Component 'vpc' in stack 'prod'").
	Label string

	// CallInfo provides additional context about what caused this node to be visited
	// (e.g., the full function call for error reporting).
	CallInfo string
}

// Context tracks the call stack during resolution to detect circular dependencies.
// It is designed to be goroutine-scoped to support concurrent resolution.
type Context struct {
	CallStack []Node
	Visited   map[string]bool
}

// contextStore maps goroutine IDs to their resolution contexts.
var contextStore sync.Map

const (
	// Initial buffer size for capturing goroutine stack traces.
	goroutineStackBufSize = 64
	// Maximum buffer size to prevent unbounded growth.
	maxGoroutineStackBufSize = 8192
)

// NewContext creates a new resolution context for cycle detection.
func NewContext() *Context {
	defer perf.Track(nil, "resolution.NewContext")()

	return &Context{
		CallStack: make([]Node, 0),
		Visited:   make(map[string]bool),
	}
}

// getGoroutineID returns the current goroutine ID.
// Returns "unknown" if parsing fails to prevent panics.
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
			// Safety limit reached.
			return "unknown"
		}
		buf = make([]byte, len(buf)*2)
	}

	// Format: "goroutine 123 [running]:\n...".
	// Parse defensively to avoid panics.
	fields := strings.Fields(string(buf))
	if len(fields) < 2 {
		return "unknown"
	}

	// Extract the number after "goroutine ".
	return fields[1]
}

// GetOrCreateContext gets or creates a resolution context for the current goroutine.
func GetOrCreateContext() *Context {
	defer perf.Track(nil, "resolution.GetOrCreateContext")()

	gid := getGoroutineID()

	if ctx, ok := contextStore.Load(gid); ok {
		return ctx.(*Context)
	}

	ctx := NewContext()
	contextStore.Store(gid, ctx)
	return ctx
}

// ClearContext clears the resolution context for the current goroutine.
func ClearContext() {
	defer perf.Track(nil, "resolution.ClearContext")()

	gid := getGoroutineID()
	contextStore.Delete(gid)
}

// ScopedContext creates a new scoped resolution context and returns a restore function.
// This prevents memory leaks and cross-call contamination by ensuring contexts are cleaned up.
// Usage:
//
//	restoreCtx := ScopedContext()
//	defer restoreCtx()
func ScopedContext() func() {
	defer perf.Track(nil, "resolution.ScopedContext")()

	gid := getGoroutineID()

	// Save the existing context (if any).
	var savedCtx *Context
	if ctx, ok := contextStore.Load(gid); ok {
		savedCtx = ctx.(*Context)
	}

	// Install a fresh context.
	freshCtx := NewContext()
	contextStore.Store(gid, freshCtx)

	// Return a restore function that reinstates the saved context or clears it.
	return func() {
		if savedCtx != nil {
			contextStore.Store(gid, savedCtx)
		} else {
			contextStore.Delete(gid)
		}
	}
}

// Push adds a node to the call stack and checks for circular dependencies.
// Returns ErrCycleDetected wrapped with details if a cycle is detected.
func (ctx *Context) Push(node Node) error {
	defer perf.Track(nil, "resolution.Context.Push")()

	// Check if we've already visited this node.
	if ctx.Visited[node.Key] {
		return ctx.buildCycleError(node)
	}

	// Mark as visited and add to call stack.
	ctx.Visited[node.Key] = true
	ctx.CallStack = append(ctx.CallStack, node)

	return nil
}

// Pop removes the top node from the call stack.
func (ctx *Context) Pop() {
	defer perf.Track(nil, "resolution.Context.Pop")()

	if len(ctx.CallStack) > 0 {
		lastIdx := len(ctx.CallStack) - 1
		node := ctx.CallStack[lastIdx]

		// Remove from visited set.
		delete(ctx.Visited, node.Key)

		// Remove from call stack.
		ctx.CallStack = ctx.CallStack[:lastIdx]
	}
}

// buildCycleError creates a detailed error message showing the dependency chain.
func (ctx *Context) buildCycleError(newNode Node) error {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("%s\n\n", ErrCycleDetected))
	builder.WriteString("Dependency chain:\n")

	// Show the full call stack.
	for i, node := range ctx.CallStack {
		builder.WriteString(fmt.Sprintf("  %d. %s\n", i+1, node.Label))
		if node.CallInfo != "" {
			builder.WriteString(fmt.Sprintf("     -> %s\n", node.CallInfo))
		}
	}

	// Show where the cycle completes.
	builder.WriteString(fmt.Sprintf("  %d. %s (cycle detected)\n",
		len(ctx.CallStack)+1, newNode.Label))
	if newNode.CallInfo != "" {
		builder.WriteString(fmt.Sprintf("     -> %s\n", newNode.CallInfo))
	}

	return fmt.Errorf("%w: %s", ErrCycleDetected, builder.String())
}

// Clone creates a copy of the resolution context for use in concurrent operations.
func (ctx *Context) Clone() *Context {
	defer perf.Track(nil, "resolution.Context.Clone")()

	if ctx == nil {
		return nil
	}

	newCtx := &Context{
		CallStack: make([]Node, len(ctx.CallStack)),
		Visited:   make(map[string]bool, len(ctx.Visited)),
	}

	copy(newCtx.CallStack, ctx.CallStack)
	for k, v := range ctx.Visited {
		newCtx.Visited[k] = v
	}

	return newCtx
}

// Len returns the number of nodes in the call stack.
func (ctx *Context) Len() int {
	defer perf.Track(nil, "resolution.Context.Len")()

	return len(ctx.CallStack)
}

// IsEmpty returns true if the call stack is empty.
func (ctx *Context) IsEmpty() bool {
	defer perf.Track(nil, "resolution.Context.IsEmpty")()

	return len(ctx.CallStack) == 0
}
