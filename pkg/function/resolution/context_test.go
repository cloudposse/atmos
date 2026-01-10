package resolution

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewContext(t *testing.T) {
	ctx := NewContext()

	assert.NotNil(t, ctx)
	assert.NotNil(t, ctx.CallStack)
	assert.NotNil(t, ctx.Visited)
	assert.Equal(t, 0, len(ctx.CallStack))
	assert.Equal(t, 0, len(ctx.Visited))
}

func TestContextPushPop(t *testing.T) {
	ctx := NewContext()

	node1 := Node{
		Key:      "dev-vpc",
		Label:    "Component 'vpc' in stack 'dev'",
		CallInfo: "!terraform.state vpc dev output",
	}

	// Push first node.
	err := ctx.Push(node1)
	require.NoError(t, err)

	assert.Equal(t, 1, len(ctx.CallStack))
	assert.Equal(t, 1, len(ctx.Visited))
	assert.True(t, ctx.Visited["dev-vpc"])

	node2 := Node{
		Key:      "dev-rds",
		Label:    "Component 'rds' in stack 'dev'",
		CallInfo: "!terraform.state rds dev output",
	}

	// Push second node.
	err = ctx.Push(node2)
	require.NoError(t, err)

	assert.Equal(t, 2, len(ctx.CallStack))
	assert.Equal(t, 2, len(ctx.Visited))
	assert.True(t, ctx.Visited["dev-rds"])

	// Pop second node.
	ctx.Pop()

	assert.Equal(t, 1, len(ctx.CallStack))
	assert.Equal(t, 1, len(ctx.Visited))
	assert.False(t, ctx.Visited["dev-rds"])
	assert.True(t, ctx.Visited["dev-vpc"])

	// Pop first node.
	ctx.Pop()

	assert.Equal(t, 0, len(ctx.CallStack))
	assert.Equal(t, 0, len(ctx.Visited))
}

func TestContextDetectsDirectCycle(t *testing.T) {
	ctx := NewContext()

	node1 := Node{
		Key:      "core-vpc",
		Label:    "Component 'vpc' in stack 'core'",
		CallInfo: "!terraform.state vpc staging output",
	}

	// Push node1 - should succeed.
	err := ctx.Push(node1)
	require.NoError(t, err)

	// Try to push node1 again - should detect cycle.
	err = ctx.Push(node1)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrCycleDetected))

	errMsg := err.Error()
	assert.Contains(t, errMsg, "cycle detected")
	assert.Contains(t, errMsg, "vpc")
	assert.Contains(t, errMsg, "core")
}

func TestContextDetectsIndirectCycle(t *testing.T) {
	ctx := NewContext()

	nodeA := Node{
		Key:      "stack-a-component-a",
		Label:    "Component 'component-a' in stack 'stack-a'",
		CallInfo: "!terraform.state component-b stack-b output",
	}

	nodeB := Node{
		Key:      "stack-b-component-b",
		Label:    "Component 'component-b' in stack 'stack-b'",
		CallInfo: "!terraform.state component-c stack-c output",
	}

	nodeC := Node{
		Key:      "stack-c-component-c",
		Label:    "Component 'component-c' in stack 'stack-c'",
		CallInfo: "!terraform.state component-a stack-a output",
	}

	// Push A -> B -> C.
	require.NoError(t, ctx.Push(nodeA))
	require.NoError(t, ctx.Push(nodeB))
	require.NoError(t, ctx.Push(nodeC))

	// Try to push A again - should detect cycle.
	err := ctx.Push(nodeA)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrCycleDetected))

	errMsg := err.Error()
	assert.Contains(t, errMsg, "component-a")
	assert.Contains(t, errMsg, "component-b")
	assert.Contains(t, errMsg, "component-c")
}

func TestContextErrorMessage(t *testing.T) {
	ctx := NewContext()

	node1 := Node{
		Key:      "core-vpc",
		Label:    "Component 'vpc' in stack 'core'",
		CallInfo: "!terraform.state vpc staging attachment_ids",
	}

	node2 := Node{
		Key:      "staging-vpc",
		Label:    "Component 'vpc' in stack 'staging'",
		CallInfo: "!terraform.state vpc core transit_gateway_id",
	}

	// Create cycle: core -> staging -> core.
	require.NoError(t, ctx.Push(node1))
	require.NoError(t, ctx.Push(node2))

	err := ctx.Push(node1)
	require.Error(t, err)

	errMsg := err.Error()

	// Check error message structure.
	assert.Contains(t, errMsg, "Dependency chain:")
	assert.Contains(t, errMsg, "1. Component 'vpc' in stack 'core'")
	assert.Contains(t, errMsg, "2. Component 'vpc' in stack 'staging'")
	assert.Contains(t, errMsg, "3. Component 'vpc' in stack 'core' (cycle detected)")

	// Check function calls are shown.
	assert.Contains(t, errMsg, "!terraform.state vpc staging attachment_ids")
	assert.Contains(t, errMsg, "!terraform.state vpc core transit_gateway_id")
}

func TestContextAllowsValidChain(t *testing.T) {
	ctx := NewContext()

	// Create valid dependency chain: A -> B -> C (no cycle).
	nodeA := Node{
		Key:      "stack-component-a",
		Label:    "Component 'component-a' in stack 'stack'",
		CallInfo: "!terraform.state component-b stack output",
	}

	nodeB := Node{
		Key:      "stack-component-b",
		Label:    "Component 'component-b' in stack 'stack'",
		CallInfo: "!terraform.state component-c stack output",
	}

	nodeC := Node{
		Key:      "stack-component-c",
		Label:    "Component 'component-c' in stack 'stack'",
		CallInfo: "!terraform.state component-d stack output",
	}

	// All pushes should succeed.
	require.NoError(t, ctx.Push(nodeA))
	require.NoError(t, ctx.Push(nodeB))
	require.NoError(t, ctx.Push(nodeC))

	// Verify state.
	assert.Equal(t, 3, len(ctx.CallStack))
	assert.Equal(t, 3, len(ctx.Visited))

	// Pop and verify cleanup.
	ctx.Pop()
	ctx.Pop()
	ctx.Pop()

	assert.Equal(t, 0, len(ctx.CallStack))
	assert.Equal(t, 0, len(ctx.Visited))
}

func TestContextClone(t *testing.T) {
	ctx := NewContext()

	node1 := Node{
		Key:      "dev-vpc",
		Label:    "Component 'vpc' in stack 'dev'",
		CallInfo: "!terraform.state vpc dev output",
	}

	require.NoError(t, ctx.Push(node1))

	// Clone the context.
	cloned := ctx.Clone()

	// Verify cloned context has same state.
	assert.Equal(t, len(ctx.CallStack), len(cloned.CallStack))
	assert.Equal(t, len(ctx.Visited), len(cloned.Visited))
	assert.Equal(t, ctx.CallStack[0], cloned.CallStack[0])
	assert.True(t, cloned.Visited["dev-vpc"])

	// Modify original - should not affect clone.
	node2 := Node{
		Key:      "dev-rds",
		Label:    "Component 'rds' in stack 'dev'",
		CallInfo: "!terraform.state rds dev output",
	}
	require.NoError(t, ctx.Push(node2))

	assert.Equal(t, 2, len(ctx.CallStack))
	assert.Equal(t, 1, len(cloned.CallStack))
}

func TestContextCloneNil(t *testing.T) {
	var ctx *Context
	cloned := ctx.Clone()
	assert.Nil(t, cloned)
}

func TestGetOrCreateContext(t *testing.T) {
	// Clear any existing context.
	ClearContext()
	defer ClearContext()

	// First call should create new context.
	ctx1 := GetOrCreateContext()
	assert.NotNil(t, ctx1)
	assert.Equal(t, 0, len(ctx1.CallStack))

	// Second call should return same context.
	ctx2 := GetOrCreateContext()
	assert.Equal(t, ctx1, ctx2)

	// Add something to verify it's the same instance.
	node := Node{
		Key:      "test-test",
		Label:    "Test node",
		CallInfo: "test",
	}
	require.NoError(t, ctx1.Push(node))

	ctx3 := GetOrCreateContext()
	assert.Equal(t, 1, len(ctx3.CallStack))
}

func TestClearContext(t *testing.T) {
	// Clear any existing context.
	ClearContext()

	// Create and populate context.
	ctx := GetOrCreateContext()
	node := Node{
		Key:      "test-test",
		Label:    "Test node",
		CallInfo: "test",
	}
	require.NoError(t, ctx.Push(node))

	// Clear context.
	ClearContext()

	// Next call should create new empty context.
	newCtx := GetOrCreateContext()
	assert.NotEqual(t, ctx, newCtx)
	assert.Equal(t, 0, len(newCtx.CallStack))
}

func TestGetGoroutineID(t *testing.T) {
	gid := getGoroutineID()

	// Should be a numeric string.
	assert.NotEmpty(t, gid)
	assert.True(t, len(gid) > 0)

	// Should be parseable as number.
	for _, c := range gid {
		assert.True(t, c >= '0' && c <= '9', "Goroutine ID should be numeric")
	}

	// Different goroutines should have different IDs.
	done := make(chan string, 1)
	go func() {
		done <- getGoroutineID()
	}()

	otherGID := <-done
	assert.NotEqual(t, gid, otherGID, "Different goroutines should have different IDs")
}

func TestContextPopEmptyStack(t *testing.T) {
	ctx := NewContext()

	// Pop from empty stack should not panic.
	assert.NotPanics(t, func() {
		ctx.Pop()
	})

	assert.Equal(t, 0, len(ctx.CallStack))
	assert.Equal(t, 0, len(ctx.Visited))
}

func TestContextMultipleStacksSameComponent(t *testing.T) {
	ctx := NewContext()

	// Same component in different stacks should be allowed (different keys).
	node1 := Node{
		Key:      "dev-vpc",
		Label:    "Component 'vpc' in stack 'dev'",
		CallInfo: "!terraform.state vpc staging output",
	}

	node2 := Node{
		Key:      "staging-vpc",
		Label:    "Component 'vpc' in stack 'staging'",
		CallInfo: "!terraform.state other staging output",
	}

	require.NoError(t, ctx.Push(node1))
	require.NoError(t, ctx.Push(node2))

	assert.Equal(t, 2, len(ctx.CallStack))
	assert.True(t, ctx.Visited["dev-vpc"])
	assert.True(t, ctx.Visited["staging-vpc"])
}

func TestContextDiamondDependency(t *testing.T) {
	// Diamond pattern: A depends on B and C, both B and C depend on D.
	// This should be allowed (not a cycle).
	ctx := NewContext()

	nodeA := Node{Key: "s-a", Label: "A", CallInfo: "test"}
	nodeB := Node{Key: "s-b", Label: "B", CallInfo: "test"}
	nodeD := Node{Key: "s-d", Label: "D", CallInfo: "test"}

	// Path: A -> B -> D.
	require.NoError(t, ctx.Push(nodeA))
	require.NoError(t, ctx.Push(nodeB))
	require.NoError(t, ctx.Push(nodeD))

	// This represents completing the B->D branch, now we'll test A->C->D.
	ctx.Pop() // Pop D.
	ctx.Pop() // Pop B.

	// Now test A -> C -> D path (C would reference D again).
	nodeC := Node{Key: "s-c", Label: "C", CallInfo: "test"}
	require.NoError(t, ctx.Push(nodeC))
	require.NoError(t, ctx.Push(nodeD)) // D is allowed again after being popped.

	assert.Equal(t, 3, len(ctx.CallStack))
}

func TestBuildCycleErrorFormatting(t *testing.T) {
	ctx := NewContext()

	// Build a 3-node cycle to test error formatting.
	node1 := Node{
		Key:      "prod-alpha",
		Label:    "Component 'alpha' in stack 'prod'",
		CallInfo: "!terraform.output beta prod value",
	}
	node2 := Node{
		Key:      "prod-beta",
		Label:    "Component 'beta' in stack 'prod'",
		CallInfo: "!terraform.state gamma prod value",
	}
	node3 := Node{
		Key:      "prod-gamma",
		Label:    "Component 'gamma' in stack 'prod'",
		CallInfo: "atmos.Component(\"alpha\", \"prod\")",
	}

	require.NoError(t, ctx.Push(node1))
	require.NoError(t, ctx.Push(node2))
	require.NoError(t, ctx.Push(node3))

	// Try to create cycle.
	err := ctx.Push(node1)
	require.Error(t, err)

	errMsg := err.Error()
	lines := strings.Split(errMsg, "\n")

	// Should have proper structure.
	var hasHeader, hasChain bool
	for _, line := range lines {
		if strings.Contains(line, "Dependency chain:") {
			hasChain = true
		}
		if strings.Contains(line, "cycle detected") {
			hasHeader = true
		}
	}

	assert.True(t, hasHeader, "Error should have cycle detected header")
	assert.True(t, hasChain, "Error should have dependency chain section")
}

func TestGoroutineLocalContextIsolation(t *testing.T) {
	// Clear any existing contexts.
	ClearContext()
	defer ClearContext()

	// Main goroutine context.
	ctx1 := GetOrCreateContext()

	node1 := Node{
		Key:      "main-stack-main-component",
		Label:    "Main component",
		CallInfo: "test",
	}
	require.NoError(t, ctx1.Push(node1))

	// Spawn another goroutine and verify it gets its own context.
	done := make(chan bool)
	go func() {
		defer close(done)

		// This goroutine should get its own context.
		ctx2 := GetOrCreateContext()

		// Should be empty initially.
		assert.Equal(t, 0, len(ctx2.CallStack))

		// Add something to this goroutine's context.
		node2 := Node{
			Key:      "goroutine-stack-goroutine-component",
			Label:    "Goroutine component",
			CallInfo: "test",
		}
		require.NoError(t, ctx2.Push(node2))

		// Should have 1 item.
		assert.Equal(t, 1, len(ctx2.CallStack))
		assert.Equal(t, "Goroutine component", ctx2.CallStack[0].Label)

		// Clean up this goroutine's context.
		ClearContext()
	}()

	<-done

	// Main goroutine's context should still have its item.
	assert.Equal(t, 1, len(ctx1.CallStack))
	assert.Equal(t, "Main component", ctx1.CallStack[0].Label)
}

func TestGetOrCreateContextMultipleCalls(t *testing.T) {
	ClearContext()
	defer ClearContext()

	// Multiple calls should return the same instance.
	ctx1 := GetOrCreateContext()
	ctx2 := GetOrCreateContext()
	ctx3 := GetOrCreateContext()

	assert.Same(t, ctx1, ctx2)
	assert.Same(t, ctx2, ctx3)
}

func TestNewContextInitialization(t *testing.T) {
	ctx := NewContext()

	// Check all fields are properly initialized.
	assert.NotNil(t, ctx, "Context should not be nil")
	assert.NotNil(t, ctx.CallStack, "CallStack should be initialized")
	assert.NotNil(t, ctx.Visited, "Visited map should be initialized")
	assert.Equal(t, 0, len(ctx.CallStack), "CallStack should be empty")
	assert.Equal(t, 0, len(ctx.Visited), "Visited should be empty")
	assert.Equal(t, 0, cap(ctx.CallStack), "CallStack should have zero capacity initially")
}

func TestContextLenAndIsEmpty(t *testing.T) {
	ctx := NewContext()

	assert.True(t, ctx.IsEmpty())
	assert.Equal(t, 0, ctx.Len())

	node := Node{Key: "test", Label: "Test", CallInfo: "test"}
	require.NoError(t, ctx.Push(node))

	assert.False(t, ctx.IsEmpty())
	assert.Equal(t, 1, ctx.Len())

	ctx.Pop()

	assert.True(t, ctx.IsEmpty())
	assert.Equal(t, 0, ctx.Len())
}

func TestScopedContext(t *testing.T) {
	ClearContext()
	defer ClearContext()

	// Set up initial context with a node.
	ctx := GetOrCreateContext()
	node := Node{Key: "outer", Label: "Outer", CallInfo: "outer call"}
	require.NoError(t, ctx.Push(node))
	assert.Equal(t, 1, ctx.Len())

	// Create scoped context.
	restore := ScopedContext()

	// New context should be fresh.
	scopedCtx := GetOrCreateContext()
	assert.Equal(t, 0, scopedCtx.Len())

	// Add nodes to scoped context.
	innerNode := Node{Key: "inner", Label: "Inner", CallInfo: "inner call"}
	require.NoError(t, scopedCtx.Push(innerNode))
	assert.Equal(t, 1, scopedCtx.Len())

	// Restore original context.
	restore()

	// Original context should be back.
	restoredCtx := GetOrCreateContext()
	assert.Equal(t, 1, restoredCtx.Len())
	assert.Equal(t, "outer", restoredCtx.CallStack[0].Key)
}

func TestScopedContextWithNoExisting(t *testing.T) {
	ClearContext()

	// Create scoped context when there's no existing context.
	restore := ScopedContext()

	// New context should be fresh.
	scopedCtx := GetOrCreateContext()
	assert.Equal(t, 0, scopedCtx.Len())

	// Add nodes to scoped context.
	node := Node{Key: "scoped", Label: "Scoped", CallInfo: "scoped call"}
	require.NoError(t, scopedCtx.Push(node))

	// Restore (should clear since there was no original).
	restore()

	// Context should be cleared, but GetOrCreateContext will create a new one.
	newCtx := GetOrCreateContext()
	assert.Equal(t, 0, newCtx.Len())

	ClearContext()
}

func TestNodeWithEmptyCallInfo(t *testing.T) {
	ctx := NewContext()

	node := Node{
		Key:      "test-key",
		Label:    "Test Label",
		CallInfo: "", // Empty call info.
	}

	require.NoError(t, ctx.Push(node))

	// Try to create cycle.
	err := ctx.Push(node)
	require.Error(t, err)

	errMsg := err.Error()
	assert.Contains(t, errMsg, "Test Label")
	// CallInfo arrow should not appear for empty call info.
	assert.NotContains(t, errMsg, "->")
}

func TestErrorIsChain(t *testing.T) {
	ctx := NewContext()

	node := Node{Key: "test", Label: "Test", CallInfo: "test"}
	require.NoError(t, ctx.Push(node))

	err := ctx.Push(node)
	require.Error(t, err)

	// Should be able to check with errors.Is.
	assert.True(t, errors.Is(err, ErrCycleDetected))

	// Should contain wrapped error.
	assert.ErrorIs(t, err, ErrCycleDetected)
}
