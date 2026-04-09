package resolution

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewContext(t *testing.T) {
	ctx := NewContext()

	require.NotNil(t, ctx)
	assert.Empty(t, ctx.CallStack)
	assert.NotNil(t, ctx.Visited)
	assert.Empty(t, ctx.Visited)
}

func TestContext_Push_Success(t *testing.T) {
	ctx := NewContext()
	atmosConfig := &schema.AtmosConfiguration{}

	node := DependencyNode{
		Component:    "vpc",
		Stack:        "tenant1-ue2-dev",
		FunctionType: "terraform.output",
		FunctionCall: "!terraform.output vpc outputs",
	}

	err := ctx.Push(atmosConfig, node)
	require.NoError(t, err)

	assert.Len(t, ctx.CallStack, 1)
	assert.Equal(t, node, ctx.CallStack[0])
	assert.True(t, ctx.Visited["tenant1-ue2-dev-vpc"])
}

func TestContext_Push_CircularDependency(t *testing.T) {
	ctx := NewContext()
	atmosConfig := &schema.AtmosConfiguration{}

	node1 := DependencyNode{
		Component:    "vpc",
		Stack:        "tenant1-ue2-dev",
		FunctionType: "terraform.output",
		FunctionCall: "!terraform.output vpc outputs",
	}

	node2 := DependencyNode{
		Component:    "eks",
		Stack:        "tenant1-ue2-dev",
		FunctionType: "terraform.output",
		FunctionCall: "!terraform.output eks cluster_name",
	}

	// Push first node.
	err := ctx.Push(atmosConfig, node1)
	require.NoError(t, err)

	// Push second node.
	err = ctx.Push(atmosConfig, node2)
	require.NoError(t, err)

	// Try to push first node again - should detect cycle.
	err = ctx.Push(atmosConfig, node1)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCircularDependency)
	assert.Contains(t, err.Error(), "vpc")
	assert.Contains(t, err.Error(), "tenant1-ue2-dev")
	assert.Contains(t, err.Error(), "cycle detected")
}

func TestContext_Pop(t *testing.T) {
	ctx := NewContext()
	atmosConfig := &schema.AtmosConfiguration{}

	node1 := DependencyNode{
		Component:    "vpc",
		Stack:        "tenant1-ue2-dev",
		FunctionType: "terraform.output",
		FunctionCall: "!terraform.output vpc outputs",
	}

	node2 := DependencyNode{
		Component:    "eks",
		Stack:        "tenant1-ue2-dev",
		FunctionType: "terraform.output",
		FunctionCall: "!terraform.output eks cluster_name",
	}

	// Push two nodes.
	require.NoError(t, ctx.Push(atmosConfig, node1))
	require.NoError(t, ctx.Push(atmosConfig, node2))

	assert.Len(t, ctx.CallStack, 2)
	assert.True(t, ctx.Visited["tenant1-ue2-dev-vpc"])
	assert.True(t, ctx.Visited["tenant1-ue2-dev-eks"])

	// Pop the second node.
	ctx.Pop(atmosConfig)

	assert.Len(t, ctx.CallStack, 1)
	assert.True(t, ctx.Visited["tenant1-ue2-dev-vpc"])
	assert.False(t, ctx.Visited["tenant1-ue2-dev-eks"])

	// Pop the first node.
	ctx.Pop(atmosConfig)

	assert.Empty(t, ctx.CallStack)
	assert.False(t, ctx.Visited["tenant1-ue2-dev-vpc"])
}

func TestContext_Pop_EmptyStack(t *testing.T) {
	ctx := NewContext()
	atmosConfig := &schema.AtmosConfiguration{}

	// Pop on empty stack should not panic.
	ctx.Pop(atmosConfig)

	assert.Empty(t, ctx.CallStack)
}

func TestContext_Clone(t *testing.T) {
	ctx := NewContext()
	atmosConfig := &schema.AtmosConfiguration{}

	node := DependencyNode{
		Component:    "vpc",
		Stack:        "tenant1-ue2-dev",
		FunctionType: "terraform.output",
		FunctionCall: "!terraform.output vpc outputs",
	}

	require.NoError(t, ctx.Push(atmosConfig, node))

	// Clone the context.
	cloned := ctx.Clone()

	// Verify cloned has same data.
	require.NotNil(t, cloned)
	assert.Len(t, cloned.CallStack, 1)
	assert.Equal(t, node, cloned.CallStack[0])
	assert.True(t, cloned.Visited["tenant1-ue2-dev-vpc"])

	// Verify cloned is independent.
	cloned.Pop(atmosConfig)
	assert.Empty(t, cloned.CallStack)
	assert.Len(t, ctx.CallStack, 1) // Original unchanged.
}

func TestContext_Clone_Nil(t *testing.T) {
	var ctx *Context
	cloned := ctx.Clone()
	assert.Nil(t, cloned)
}

func TestGetOrCreate(t *testing.T) {
	// Clear any existing context.
	Clear()

	// First call should create a new context.
	ctx1 := GetOrCreate()
	require.NotNil(t, ctx1)
	assert.Empty(t, ctx1.CallStack)

	// Add a node to the context.
	atmosConfig := &schema.AtmosConfiguration{}
	node := DependencyNode{
		Component:    "vpc",
		Stack:        "test",
		FunctionType: "terraform.output",
		FunctionCall: "test",
	}
	require.NoError(t, ctx1.Push(atmosConfig, node))

	// Second call should return the same context.
	ctx2 := GetOrCreate()
	assert.Same(t, ctx1, ctx2)
	assert.Len(t, ctx2.CallStack, 1)

	// Cleanup.
	Clear()
}

func TestClear(t *testing.T) {
	// Create a context.
	ctx := GetOrCreate()
	atmosConfig := &schema.AtmosConfiguration{}

	node := DependencyNode{
		Component:    "vpc",
		Stack:        "test",
		FunctionType: "terraform.output",
		FunctionCall: "test",
	}
	require.NoError(t, ctx.Push(atmosConfig, node))

	// Clear the context.
	Clear()

	// Next GetOrCreate should return a fresh context.
	newCtx := GetOrCreate()
	assert.Empty(t, newCtx.CallStack)

	// Cleanup.
	Clear()
}

func TestScoped(t *testing.T) {
	// Setup an existing context.
	existingCtx := GetOrCreate()
	atmosConfig := &schema.AtmosConfiguration{}

	node := DependencyNode{
		Component:    "vpc",
		Stack:        "test",
		FunctionType: "terraform.output",
		FunctionCall: "test",
	}
	require.NoError(t, existingCtx.Push(atmosConfig, node))

	// Create a scoped context.
	restore := Scoped()

	// Within the scope, we should have a fresh context.
	scopedCtx := GetOrCreate()
	assert.Empty(t, scopedCtx.CallStack)
	assert.NotSame(t, existingCtx, scopedCtx)

	// Add something to the scoped context.
	node2 := DependencyNode{
		Component:    "eks",
		Stack:        "test",
		FunctionType: "terraform.output",
		FunctionCall: "test2",
	}
	require.NoError(t, scopedCtx.Push(atmosConfig, node2))

	// Restore the original context.
	restore()

	// After restore, we should have the original context back.
	restoredCtx := GetOrCreate()
	assert.Len(t, restoredCtx.CallStack, 1)
	assert.Equal(t, "vpc", restoredCtx.CallStack[0].Component)

	// Cleanup.
	Clear()
}

func TestScoped_NoExistingContext(t *testing.T) {
	// Make sure there's no existing context.
	Clear()

	// Create a scoped context.
	restore := Scoped()

	// Add something to the scoped context.
	ctx := GetOrCreate()
	atmosConfig := &schema.AtmosConfiguration{}
	node := DependencyNode{
		Component:    "test",
		Stack:        "test",
		FunctionType: "test",
		FunctionCall: "test",
	}
	require.NoError(t, ctx.Push(atmosConfig, node))

	// Restore.
	restore()

	// After restore, there should be no context (it was nil before).
	// Calling GetOrCreate should create a fresh one.
	newCtx := GetOrCreate()
	assert.Empty(t, newCtx.CallStack)

	// Cleanup.
	Clear()
}

func TestGetGoroutineID(t *testing.T) {
	// Test that we get a consistent ID within the same goroutine.
	id1 := getGoroutineID()
	id2 := getGoroutineID()

	assert.Equal(t, id1, id2)
	assert.NotEmpty(t, id1)
}

func TestGetGoroutineID_DifferentGoroutines(t *testing.T) {
	var wg sync.WaitGroup
	ids := make([]string, 2)

	wg.Add(2)

	go func() {
		defer wg.Done()
		ids[0] = getGoroutineID()
	}()

	go func() {
		defer wg.Done()
		ids[1] = getGoroutineID()
	}()

	wg.Wait()

	// Different goroutines should have different IDs.
	assert.NotEmpty(t, ids[0])
	assert.NotEmpty(t, ids[1])
	assert.NotEqual(t, ids[0], ids[1])
}

func TestBuildCircularDependencyError(t *testing.T) {
	ctx := NewContext()
	atmosConfig := &schema.AtmosConfiguration{}

	// Build a call stack.
	node1 := DependencyNode{
		Component:    "vpc",
		Stack:        "tenant1-ue2-dev",
		FunctionType: "terraform.output",
		FunctionCall: "!terraform.output vpc vpc_id",
	}
	node2 := DependencyNode{
		Component:    "eks",
		Stack:        "tenant1-ue2-dev",
		FunctionType: "terraform.output",
		FunctionCall: "!terraform.output eks cluster_arn",
	}

	require.NoError(t, ctx.Push(atmosConfig, node1))
	require.NoError(t, ctx.Push(atmosConfig, node2))

	// Build the error for a circular dependency back to vpc.
	newNode := DependencyNode{
		Component:    "vpc",
		Stack:        "tenant1-ue2-dev",
		FunctionType: "terraform.output",
		FunctionCall: "!terraform.output vpc subnet_ids",
	}

	err := ctx.buildCircularDependencyError(newNode)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCircularDependency)
	assert.Contains(t, err.Error(), "Dependency chain")
	assert.Contains(t, err.Error(), "vpc")
	assert.Contains(t, err.Error(), "eks")
	assert.Contains(t, err.Error(), "tenant1-ue2-dev")
	assert.Contains(t, err.Error(), "cycle detected")
	assert.Contains(t, err.Error(), "To fix this issue")
}

func TestConcurrentContextAccess(t *testing.T) {
	var wg sync.WaitGroup
	numGoroutines := 10

	// Clear any existing contexts.
	goroutineContexts = sync.Map{}

	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()

			// Each goroutine gets/creates its own context.
			ctx := GetOrCreate()
			assert.NotNil(t, ctx)

			// Push a unique node.
			atmosConfig := &schema.AtmosConfiguration{}
			node := DependencyNode{
				Component:    "comp",
				Stack:        "stack",
				FunctionType: "terraform.output",
				FunctionCall: "test",
			}
			err := ctx.Push(atmosConfig, node)
			assert.NoError(t, err)

			// Verify the context has exactly one node.
			assert.Len(t, ctx.CallStack, 1)

			// Clear at the end.
			Clear()
		}()
	}

	wg.Wait()
}
