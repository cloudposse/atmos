package exec

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewResolutionContext(t *testing.T) {
	ctx := NewResolutionContext()

	assert.NotNil(t, ctx)
	assert.NotNil(t, ctx.CallStack)
	assert.NotNil(t, ctx.Visited)
	assert.Equal(t, 0, len(ctx.CallStack))
	assert.Equal(t, 0, len(ctx.Visited))
}

func TestResolutionContextPushPop(t *testing.T) {
	ctx := NewResolutionContext()
	atmosConfig := &schema.AtmosConfiguration{}

	node1 := DependencyNode{
		Component:    "vpc",
		Stack:        "dev",
		FunctionType: "terraform.state",
		FunctionCall: "!terraform.state vpc dev output",
	}

	// Push first node.
	err := ctx.Push(atmosConfig, node1)
	require.NoError(t, err)

	assert.Equal(t, 1, len(ctx.CallStack))
	assert.Equal(t, 1, len(ctx.Visited))
	assert.True(t, ctx.Visited["dev-vpc"])

	node2 := DependencyNode{
		Component:    "rds",
		Stack:        "dev",
		FunctionType: "terraform.state",
		FunctionCall: "!terraform.state rds dev output",
	}

	// Push second node.
	err = ctx.Push(atmosConfig, node2)
	require.NoError(t, err)

	assert.Equal(t, 2, len(ctx.CallStack))
	assert.Equal(t, 2, len(ctx.Visited))
	assert.True(t, ctx.Visited["dev-rds"])

	// Pop second node.
	ctx.Pop(atmosConfig)

	assert.Equal(t, 1, len(ctx.CallStack))
	assert.Equal(t, 1, len(ctx.Visited))
	assert.False(t, ctx.Visited["dev-rds"])
	assert.True(t, ctx.Visited["dev-vpc"])

	// Pop first node.
	ctx.Pop(atmosConfig)

	assert.Equal(t, 0, len(ctx.CallStack))
	assert.Equal(t, 0, len(ctx.Visited))
}

func TestResolutionContextDetectsDirectCycle(t *testing.T) {
	ctx := NewResolutionContext()
	atmosConfig := &schema.AtmosConfiguration{}

	node1 := DependencyNode{
		Component:    "vpc",
		Stack:        "core",
		FunctionType: "terraform.state",
		FunctionCall: "!terraform.state vpc staging output",
	}

	// Push node1 - should succeed.
	err := ctx.Push(atmosConfig, node1)
	require.NoError(t, err)

	// Try to push node1 again - should detect cycle.
	err = ctx.Push(atmosConfig, node1)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCircularDependency)

	errMsg := err.Error()
	assert.Contains(t, errMsg, "circular dependency")
	assert.Contains(t, errMsg, "vpc")
	assert.Contains(t, errMsg, "core")
}

func TestResolutionContextDetectsIndirectCycle(t *testing.T) {
	ctx := NewResolutionContext()
	atmosConfig := &schema.AtmosConfiguration{}

	nodeA := DependencyNode{
		Component:    "component-a",
		Stack:        "stack-a",
		FunctionType: "terraform.state",
		FunctionCall: "!terraform.state component-b stack-b output",
	}

	nodeB := DependencyNode{
		Component:    "component-b",
		Stack:        "stack-b",
		FunctionType: "terraform.state",
		FunctionCall: "!terraform.state component-c stack-c output",
	}

	nodeC := DependencyNode{
		Component:    "component-c",
		Stack:        "stack-c",
		FunctionType: "terraform.state",
		FunctionCall: "!terraform.state component-a stack-a output",
	}

	// Push A -> B -> C.
	require.NoError(t, ctx.Push(atmosConfig, nodeA))
	require.NoError(t, ctx.Push(atmosConfig, nodeB))
	require.NoError(t, ctx.Push(atmosConfig, nodeC))

	// Try to push A again - should detect cycle.
	err := ctx.Push(atmosConfig, nodeA)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCircularDependency)

	errMsg := err.Error()
	assert.Contains(t, errMsg, "component-a")
	assert.Contains(t, errMsg, "component-b")
	assert.Contains(t, errMsg, "component-c")
}

func TestResolutionContextErrorMessage(t *testing.T) {
	ctx := NewResolutionContext()
	atmosConfig := &schema.AtmosConfiguration{}

	node1 := DependencyNode{
		Component:    "vpc",
		Stack:        "core",
		FunctionType: "terraform.state",
		FunctionCall: "!terraform.state vpc staging attachment_ids",
	}

	node2 := DependencyNode{
		Component:    "vpc",
		Stack:        "staging",
		FunctionType: "terraform.state",
		FunctionCall: "!terraform.state vpc core transit_gateway_id",
	}

	// Create cycle: core -> staging -> core.
	require.NoError(t, ctx.Push(atmosConfig, node1))
	require.NoError(t, ctx.Push(atmosConfig, node2))

	err := ctx.Push(atmosConfig, node1)
	require.Error(t, err)

	errMsg := err.Error()

	// Check error message structure.
	assert.Contains(t, errMsg, "Dependency chain:")
	assert.Contains(t, errMsg, "1. Component 'vpc' in stack 'core'")
	assert.Contains(t, errMsg, "2. Component 'vpc' in stack 'staging'")
	assert.Contains(t, errMsg, "3. Component 'vpc' in stack 'core' (cycle detected)")

	// Check fix suggestions.
	assert.Contains(t, errMsg, "To fix this issue:")
	assert.Contains(t, errMsg, "break the circular reference")
	assert.Contains(t, errMsg, "Terraform data sources")
	assert.Contains(t, errMsg, "one direction only")

	// Check function calls are shown.
	assert.Contains(t, errMsg, "!terraform.state vpc staging attachment_ids")
	assert.Contains(t, errMsg, "!terraform.state vpc core transit_gateway_id")
}

func TestResolutionContextAllowsValidChain(t *testing.T) {
	ctx := NewResolutionContext()
	atmosConfig := &schema.AtmosConfiguration{}

	// Create valid dependency chain: A -> B -> C (no cycle).
	nodeA := DependencyNode{
		Component:    "component-a",
		Stack:        "stack",
		FunctionType: "terraform.state",
		FunctionCall: "!terraform.state component-b stack output",
	}

	nodeB := DependencyNode{
		Component:    "component-b",
		Stack:        "stack",
		FunctionType: "terraform.state",
		FunctionCall: "!terraform.state component-c stack output",
	}

	nodeC := DependencyNode{
		Component:    "component-c",
		Stack:        "stack",
		FunctionType: "terraform.state",
		FunctionCall: "!terraform.state component-d stack output",
	}

	// All pushes should succeed.
	require.NoError(t, ctx.Push(atmosConfig, nodeA))
	require.NoError(t, ctx.Push(atmosConfig, nodeB))
	require.NoError(t, ctx.Push(atmosConfig, nodeC))

	// Verify state.
	assert.Equal(t, 3, len(ctx.CallStack))
	assert.Equal(t, 3, len(ctx.Visited))

	// Pop and verify cleanup.
	ctx.Pop(atmosConfig)
	ctx.Pop(atmosConfig)
	ctx.Pop(atmosConfig)

	assert.Equal(t, 0, len(ctx.CallStack))
	assert.Equal(t, 0, len(ctx.Visited))
}

func TestResolutionContextClone(t *testing.T) {
	ctx := NewResolutionContext()
	atmosConfig := &schema.AtmosConfiguration{}

	node1 := DependencyNode{
		Component:    "vpc",
		Stack:        "dev",
		FunctionType: "terraform.state",
		FunctionCall: "!terraform.state vpc dev output",
	}

	require.NoError(t, ctx.Push(atmosConfig, node1))

	// Clone the context.
	cloned := ctx.Clone()

	// Verify cloned context has same state.
	assert.Equal(t, len(ctx.CallStack), len(cloned.CallStack))
	assert.Equal(t, len(ctx.Visited), len(cloned.Visited))
	assert.Equal(t, ctx.CallStack[0], cloned.CallStack[0])
	assert.True(t, cloned.Visited["dev-vpc"])

	// Modify original - should not affect clone.
	node2 := DependencyNode{
		Component:    "rds",
		Stack:        "dev",
		FunctionType: "terraform.state",
		FunctionCall: "!terraform.state rds dev output",
	}
	require.NoError(t, ctx.Push(atmosConfig, node2))

	assert.Equal(t, 2, len(ctx.CallStack))
	assert.Equal(t, 1, len(cloned.CallStack))
}

func TestResolutionContextCloneNil(t *testing.T) {
	var ctx *ResolutionContext
	cloned := ctx.Clone()
	assert.Nil(t, cloned)
}

func TestGetOrCreateResolutionContext(t *testing.T) {
	// Clear any existing context.
	ClearResolutionContext()
	defer ClearResolutionContext()

	// First call should create new context.
	ctx1 := GetOrCreateResolutionContext()
	assert.NotNil(t, ctx1)
	assert.Equal(t, 0, len(ctx1.CallStack))

	// Second call should return same context.
	ctx2 := GetOrCreateResolutionContext()
	assert.Equal(t, ctx1, ctx2)

	// Add something to verify it's the same instance.
	atmosConfig := &schema.AtmosConfiguration{}
	node := DependencyNode{
		Component:    "test",
		Stack:        "test",
		FunctionType: "test",
		FunctionCall: "test",
	}
	require.NoError(t, ctx1.Push(atmosConfig, node))

	ctx3 := GetOrCreateResolutionContext()
	assert.Equal(t, 1, len(ctx3.CallStack))
}

func TestClearResolutionContext(t *testing.T) {
	// Clear any existing context.
	ClearResolutionContext()

	// Create and populate context.
	ctx := GetOrCreateResolutionContext()
	atmosConfig := &schema.AtmosConfiguration{}
	node := DependencyNode{
		Component:    "test",
		Stack:        "test",
		FunctionType: "test",
		FunctionCall: "test",
	}
	require.NoError(t, ctx.Push(atmosConfig, node))

	// Clear context.
	ClearResolutionContext()

	// Next call should create new empty context.
	newCtx := GetOrCreateResolutionContext()
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

func TestResolutionContextMixedFunctionTypes(t *testing.T) {
	ctx := NewResolutionContext()
	atmosConfig := &schema.AtmosConfiguration{}

	// Create cycle with mixed function types.
	node1 := DependencyNode{
		Component:    "vpc",
		Stack:        "core",
		FunctionType: "terraform.state",
		FunctionCall: "!terraform.state vpc staging output",
	}

	node2 := DependencyNode{
		Component:    "vpc",
		Stack:        "staging",
		FunctionType: "atmos.Component",
		FunctionCall: "atmos.Component(\"vpc\", \"core\")",
	}

	require.NoError(t, ctx.Push(atmosConfig, node1))
	require.NoError(t, ctx.Push(atmosConfig, node2))

	// Try to push node1 again - should detect cycle across function types.
	err := ctx.Push(atmosConfig, node1)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCircularDependency)

	errMsg := err.Error()
	assert.Contains(t, errMsg, "terraform.state")
	assert.Contains(t, errMsg, "atmos.Component")
}

func TestResolutionContextPopEmptyStack(t *testing.T) {
	ctx := NewResolutionContext()
	atmosConfig := &schema.AtmosConfiguration{}

	// Pop from empty stack should not panic.
	assert.NotPanics(t, func() {
		ctx.Pop(atmosConfig)
	})

	assert.Equal(t, 0, len(ctx.CallStack))
	assert.Equal(t, 0, len(ctx.Visited))
}

func TestResolutionContextMultipleStacksSameComponent(t *testing.T) {
	ctx := NewResolutionContext()
	atmosConfig := &schema.AtmosConfiguration{}

	// Same component in different stacks should be allowed.
	node1 := DependencyNode{
		Component:    "vpc",
		Stack:        "dev",
		FunctionType: "terraform.state",
		FunctionCall: "!terraform.state vpc staging output",
	}

	node2 := DependencyNode{
		Component:    "vpc",
		Stack:        "staging",
		FunctionType: "terraform.state",
		FunctionCall: "!terraform.state other staging output",
	}

	require.NoError(t, ctx.Push(atmosConfig, node1))
	require.NoError(t, ctx.Push(atmosConfig, node2))

	assert.Equal(t, 2, len(ctx.CallStack))
	assert.True(t, ctx.Visited["dev-vpc"])
	assert.True(t, ctx.Visited["staging-vpc"])
}

func TestResolutionContextDiamondDependency(t *testing.T) {
	// Diamond pattern: A depends on B and C, both B and C depend on D.
	// This should be allowed (not a cycle).
	ctx := NewResolutionContext()
	atmosConfig := &schema.AtmosConfiguration{}

	nodeA := DependencyNode{Component: "a", Stack: "s", FunctionType: "test", FunctionCall: "test"}
	nodeB := DependencyNode{Component: "b", Stack: "s", FunctionType: "test", FunctionCall: "test"}
	nodeD := DependencyNode{Component: "d", Stack: "s", FunctionType: "test", FunctionCall: "test"}

	// Path: A -> B -> D.
	require.NoError(t, ctx.Push(atmosConfig, nodeA))
	require.NoError(t, ctx.Push(atmosConfig, nodeB))
	require.NoError(t, ctx.Push(atmosConfig, nodeD))

	// This represents completing the B->D branch, now we'll test A->C->D.
	ctx.Pop(atmosConfig) // Pop D.
	ctx.Pop(atmosConfig) // Pop B.

	// Now test A -> C -> D path (C would reference D again).
	nodeC := DependencyNode{Component: "c", Stack: "s", FunctionType: "test", FunctionCall: "test"}
	require.NoError(t, ctx.Push(atmosConfig, nodeC))
	require.NoError(t, ctx.Push(atmosConfig, nodeD)) // D is allowed again after being popped.

	assert.Equal(t, 3, len(ctx.CallStack))
}

func TestBuildCircularDependencyErrorFormatting(t *testing.T) {
	ctx := NewResolutionContext()
	atmosConfig := &schema.AtmosConfiguration{}

	// Build a 3-node cycle to test error formatting.
	node1 := DependencyNode{
		Component:    "alpha",
		Stack:        "prod",
		FunctionType: "terraform.output",
		FunctionCall: "!terraform.output beta prod value",
	}
	node2 := DependencyNode{
		Component:    "beta",
		Stack:        "prod",
		FunctionType: "terraform.state",
		FunctionCall: "!terraform.state gamma prod value",
	}
	node3 := DependencyNode{
		Component:    "gamma",
		Stack:        "prod",
		FunctionType: "atmos.Component",
		FunctionCall: "atmos.Component(\"alpha\", \"prod\")",
	}

	require.NoError(t, ctx.Push(atmosConfig, node1))
	require.NoError(t, ctx.Push(atmosConfig, node2))
	require.NoError(t, ctx.Push(atmosConfig, node3))

	// Try to create cycle.
	err := ctx.Push(atmosConfig, node1)
	require.Error(t, err)

	errMsg := err.Error()
	lines := strings.Split(errMsg, "\n")

	// Should have proper structure.
	var hasHeader, hasChain, hasFixSuggestions bool
	for _, line := range lines {
		if strings.Contains(line, "Dependency chain:") {
			hasChain = true
		}
		if strings.Contains(line, "circular dependency") {
			hasHeader = true
		}
		if strings.Contains(line, "To fix this issue:") {
			hasFixSuggestions = true
		}
	}

	assert.True(t, hasHeader, "Error should have circular dependency header")
	assert.True(t, hasChain, "Error should have dependency chain section")
	assert.True(t, hasFixSuggestions, "Error should have fix suggestions")
}

func TestGoroutineLocalContextIsolation(t *testing.T) {
	// Clear any existing contexts.
	ClearResolutionContext()
	defer ClearResolutionContext()

	// Main goroutine context.
	ctx1 := GetOrCreateResolutionContext()
	atmosConfig := &schema.AtmosConfiguration{}

	node1 := DependencyNode{
		Component:    "main-component",
		Stack:        "main-stack",
		FunctionType: "test",
		FunctionCall: "test",
	}
	require.NoError(t, ctx1.Push(atmosConfig, node1))

	// Spawn another goroutine and verify it gets its own context.
	done := make(chan bool)
	go func() {
		defer close(done)

		// This goroutine should get its own context.
		ctx2 := GetOrCreateResolutionContext()

		// Should be empty initially.
		assert.Equal(t, 0, len(ctx2.CallStack))

		// Add something to this goroutine's context.
		node2 := DependencyNode{
			Component:    "goroutine-component",
			Stack:        "goroutine-stack",
			FunctionType: "test",
			FunctionCall: "test",
		}
		require.NoError(t, ctx2.Push(atmosConfig, node2))

		// Should have 1 item.
		assert.Equal(t, 1, len(ctx2.CallStack))
		assert.Equal(t, "goroutine-component", ctx2.CallStack[0].Component)

		// Clean up this goroutine's context.
		ClearResolutionContext()
	}()

	<-done

	// Main goroutine's context should still have its item.
	assert.Equal(t, 1, len(ctx1.CallStack))
	assert.Equal(t, "main-component", ctx1.CallStack[0].Component)
}

func TestGetOrCreateResolutionContextMultipleCalls(t *testing.T) {
	ClearResolutionContext()
	defer ClearResolutionContext()

	// Multiple calls should return the same instance.
	ctx1 := GetOrCreateResolutionContext()
	ctx2 := GetOrCreateResolutionContext()
	ctx3 := GetOrCreateResolutionContext()

	assert.Same(t, ctx1, ctx2)
	assert.Same(t, ctx2, ctx3)
}

func TestNewResolutionContextInitialization(t *testing.T) {
	ctx := NewResolutionContext()

	// Check all fields are properly initialized.
	assert.NotNil(t, ctx, "Context should not be nil")
	assert.NotNil(t, ctx.CallStack, "CallStack should be initialized")
	assert.NotNil(t, ctx.Visited, "Visited map should be initialized")
	assert.Equal(t, 0, len(ctx.CallStack), "CallStack should be empty")
	assert.Equal(t, 0, len(ctx.Visited), "Visited should be empty")
	assert.Equal(t, 0, cap(ctx.CallStack), "CallStack should have zero capacity initially")
}
