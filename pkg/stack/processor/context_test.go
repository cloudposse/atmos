package processor

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/function"
)

func TestNewStackContext(t *testing.T) {
	t.Run("with nil execution context", func(t *testing.T) {
		ctx := NewStackContext(nil)

		assert.NotNil(t, ctx)
		assert.NotNil(t, ctx.ExecutionContext)
		assert.Empty(t, ctx.CurrentStack)
		assert.Empty(t, ctx.CurrentComponent)
		assert.Empty(t, ctx.Skip)
		assert.False(t, ctx.DryRun)
	})

	t.Run("with execution context", func(t *testing.T) {
		execCtx := &function.ExecutionContext{
			Env:        map[string]string{"AWS_REGION": "us-east-1"},
			WorkingDir: "/work",
			SourceFile: "test.yaml",
		}

		ctx := NewStackContext(execCtx)

		assert.NotNil(t, ctx)
		assert.Same(t, execCtx, ctx.ExecutionContext)
		assert.Equal(t, "us-east-1", ctx.GetEnv("AWS_REGION"))
	})
}

func TestStackContextWithStack(t *testing.T) {
	ctx := NewStackContext(nil)

	result := ctx.WithStack("prod-us-east-1")

	assert.Same(t, ctx, result) // Returns same instance for chaining.
	assert.Equal(t, "prod-us-east-1", ctx.CurrentStack)
}

func TestStackContextWithComponent(t *testing.T) {
	ctx := NewStackContext(nil)

	result := ctx.WithComponent("vpc")

	assert.Same(t, ctx, result)
	assert.Equal(t, "vpc", ctx.CurrentComponent)
}

func TestStackContextWithSkip(t *testing.T) {
	ctx := NewStackContext(nil)

	t.Run("with values", func(t *testing.T) {
		result := ctx.WithSkip([]string{"terraform.output", "store.get"})

		assert.Same(t, ctx, result)
		assert.Equal(t, []string{"terraform.output", "store.get"}, ctx.Skip)
	})

	t.Run("with nil", func(t *testing.T) {
		result := ctx.WithSkip(nil)

		assert.Same(t, ctx, result)
		assert.NotNil(t, ctx.Skip)
		assert.Empty(t, ctx.Skip)
	})
}

func TestStackContextWithDryRun(t *testing.T) {
	ctx := NewStackContext(nil)

	result := ctx.WithDryRun(true)

	assert.Same(t, ctx, result)
	assert.True(t, ctx.DryRun)

	ctx.WithDryRun(false)
	assert.False(t, ctx.DryRun)
}

func TestStackContextWithStacksBasePath(t *testing.T) {
	ctx := NewStackContext(nil)

	result := ctx.WithStacksBasePath("/app/stacks")

	assert.Same(t, ctx, result)
	assert.Equal(t, "/app/stacks", ctx.StacksBasePath)
}

func TestStackContextWithComponentsBasePath(t *testing.T) {
	ctx := NewStackContext(nil)

	result := ctx.WithComponentsBasePath("/app/components")

	assert.Same(t, ctx, result)
	assert.Equal(t, "/app/components", ctx.ComponentsBasePath)
}

func TestStackContextShouldSkip(t *testing.T) {
	tests := []struct {
		name     string
		ctx      *StackContext
		funcName string
		expected bool
	}{
		{
			name:     "nil context",
			ctx:      nil,
			funcName: "env",
			expected: false,
		},
		{
			name:     "empty skip list",
			ctx:      NewStackContext(nil),
			funcName: "env",
			expected: false,
		},
		{
			name:     "function in skip list",
			ctx:      NewStackContext(nil).WithSkip([]string{"env", "exec"}),
			funcName: "env",
			expected: true,
		},
		{
			name:     "function not in skip list",
			ctx:      NewStackContext(nil).WithSkip([]string{"env", "exec"}),
			funcName: "terraform.output",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.ctx.ShouldSkip(tt.funcName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStackContextClone(t *testing.T) {
	t.Run("clone nil context", func(t *testing.T) {
		var ctx *StackContext
		clone := ctx.Clone()
		assert.Nil(t, clone)
	})

	t.Run("clone context", func(t *testing.T) {
		execCtx := &function.ExecutionContext{
			Env:        map[string]string{"AWS_REGION": "us-east-1"},
			WorkingDir: "/work",
			SourceFile: "test.yaml",
		}

		ctx := NewStackContext(execCtx).
			WithStack("prod").
			WithComponent("vpc").
			WithSkip([]string{"env"}).
			WithDryRun(true).
			WithStacksBasePath("/stacks").
			WithComponentsBasePath("/components")

		clone := ctx.Clone()

		// Verify values are equal.
		assert.Equal(t, ctx.CurrentStack, clone.CurrentStack)
		assert.Equal(t, ctx.CurrentComponent, clone.CurrentComponent)
		assert.Equal(t, ctx.Skip, clone.Skip)
		assert.Equal(t, ctx.DryRun, clone.DryRun)
		assert.Equal(t, ctx.StacksBasePath, clone.StacksBasePath)
		assert.Equal(t, ctx.ComponentsBasePath, clone.ComponentsBasePath)

		// Verify Skip slice is independent.
		clone.Skip[0] = "modified"
		assert.NotEqual(t, ctx.Skip[0], clone.Skip[0])

		// Verify they share the same ExecutionContext (shallow copy).
		assert.Same(t, ctx.ExecutionContext, clone.ExecutionContext)
	})
}

func TestStackContextChaining(t *testing.T) {
	ctx := NewStackContext(nil).
		WithStack("prod-us-east-1").
		WithComponent("vpc").
		WithSkip([]string{"env"}).
		WithDryRun(true).
		WithStacksBasePath("/stacks").
		WithComponentsBasePath("/components")

	assert.Equal(t, "prod-us-east-1", ctx.CurrentStack)
	assert.Equal(t, "vpc", ctx.CurrentComponent)
	assert.Equal(t, []string{"env"}, ctx.Skip)
	assert.True(t, ctx.DryRun)
	assert.Equal(t, "/stacks", ctx.StacksBasePath)
	assert.Equal(t, "/components", ctx.ComponentsBasePath)
}
