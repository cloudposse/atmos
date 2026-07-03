package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/tools"
)

// mockTool is a simple mock tool for testing.
type mockTool struct {
	name        string
	description string
	params      []tools.Parameter
	executeFunc func(ctx context.Context, params map[string]interface{}) (*tools.Result, error)
}

func (m *mockTool) Name() string                  { return m.name }
func (m *mockTool) Description() string           { return m.description }
func (m *mockTool) Parameters() []tools.Parameter { return m.params }
func (m *mockTool) RequiresPermission() bool      { return false }
func (m *mockTool) IsRestricted() bool            { return false }
func (m *mockTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, params)
	}
	return &tools.Result{Success: true, Output: "mock result"}, nil
}

func TestNewAdapter(t *testing.T) {
	registry := tools.NewRegistry()
	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)

	adapter := NewAdapter(registry, executor)

	assert.NotNil(t, adapter)
	assert.Equal(t, registry, adapter.registry)
	assert.Equal(t, executor, adapter.executor)
}

func TestAdapter_ExecuteTool_Success(t *testing.T) {
	registry := tools.NewRegistry()

	// Register a mock tool.
	mockTool := &mockTool{
		name:        "test_tool",
		description: "A test tool",
		executeFunc: func(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
			return &tools.Result{
				Success: true,
				Output:  "Test output",
				Data: map[string]interface{}{
					"key1": "value1",
					"key2": 42,
				},
			}, nil
		},
	}
	err := registry.Register(mockTool)
	require.NoError(t, err)

	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)

	// Execute the tool.
	ctx := context.Background()
	args := map[string]interface{}{"arg1": "value1"}

	result, err := adapter.ExecuteTool(ctx, "test_tool", args)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, "Test output", result.Output)
	assert.Equal(t, "value1", result.Data["key1"])
	assert.Equal(t, 42, result.Data["key2"])
}

func TestAdapter_ExecuteTool_ToolNotFound(t *testing.T) {
	registry := tools.NewRegistry()
	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)

	ctx := context.Background()
	args := map[string]interface{}{}

	result, err := adapter.ExecuteTool(ctx, "nonexistent_tool", args)

	// Adapter returns error as Result with Success=false.
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Success)
	assert.NotNil(t, result.Error)
	assert.Contains(t, result.Error.Error(), "tool execution failed")
}

func TestAdapter_ExecuteTool_ExecutionError(t *testing.T) {
	registry := tools.NewRegistry()

	// Register a mock tool that returns an error.
	mockTool := &mockTool{
		name:        "error_tool",
		description: "A tool that errors",
		executeFunc: func(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
			return nil, errors.New("execution failed")
		},
	}
	err := registry.Register(mockTool)
	require.NoError(t, err)

	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)

	ctx := context.Background()
	args := map[string]interface{}{}

	result, err := adapter.ExecuteTool(ctx, "error_tool", args)

	// Adapter wraps the error in Result.
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Success)
	assert.NotNil(t, result.Error)
	assert.Contains(t, result.Error.Error(), "tool execution failed")
}

func TestAdapter_ExecuteTool_WithEmptyArguments(t *testing.T) {
	registry := tools.NewRegistry()

	mockTool := &mockTool{
		name:        "no_args_tool",
		description: "A tool with no args",
		executeFunc: func(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
			return &tools.Result{
				Success: true,
				Output:  "No args needed",
			}, nil
		},
	}
	err := registry.Register(mockTool)
	require.NoError(t, err)

	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)

	ctx := context.Background()

	result, err := adapter.ExecuteTool(ctx, "no_args_tool", nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, "No args needed", result.Output)
}

func TestAdapter_ExecuteTool_ContextCancellation(t *testing.T) {
	registry := tools.NewRegistry()

	mockTool := &mockTool{
		name:        "context_tool",
		description: "A tool that checks context",
		executeFunc: func(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
				return &tools.Result{Success: true, Output: "OK"}, nil
			}
		},
	}
	err := registry.Register(mockTool)
	require.NoError(t, err)

	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)

	// Create cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := adapter.ExecuteTool(ctx, "context_tool", nil)

	// Should return error wrapped in result.
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Success)
	assert.NotNil(t, result.Error)
}
