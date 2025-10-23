package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/tools"
)

// mockTool implements tools.Tool for testing.
type mockTool struct {
	name        string
	description string
	parameters  []tools.Parameter
	executeFunc func(ctx context.Context, args map[string]interface{}) (*tools.Result, error)
}

func (m *mockTool) Name() string {
	return m.name
}

func (m *mockTool) Description() string {
	return m.description
}

func (m *mockTool) Parameters() []tools.Parameter {
	return m.parameters
}

func (m *mockTool) Execute(ctx context.Context, args map[string]interface{}) (*tools.Result, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, args)
	}
	return &tools.Result{
		Success: true,
		Output:  "mock output",
	}, nil
}

func (m *mockTool) RequiresPermission() bool {
	return false
}

func (m *mockTool) IsRestricted() bool {
	return false
}

func TestNewAdapter(t *testing.T) {
	registry := tools.NewRegistry()
	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)

	adapter := NewAdapter(registry, executor)
	assert.NotNil(t, adapter)
	assert.Equal(t, registry, adapter.registry)
	assert.Equal(t, executor, adapter.executor)
}

func TestListTools(t *testing.T) {
	registry := tools.NewRegistry()

	// Register test tools.
	tool1 := &mockTool{
		name:        "test_tool_1",
		description: "First test tool",
		parameters: []tools.Parameter{
			{
				Name:        "arg1",
				Description: "First argument",
				Type:        tools.ParamTypeString,
				Required:    true,
			},
		},
	}

	tool2 := &mockTool{
		name:        "test_tool_2",
		description: "Second test tool",
		parameters: []tools.Parameter{
			{
				Name:        "count",
				Description: "Count value",
				Type:        tools.ParamTypeInt,
				Required:    false,
				Default:     10,
			},
		},
	}

	err := registry.Register(tool1)
	require.NoError(t, err)
	err = registry.Register(tool2)
	require.NoError(t, err)

	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)

	mcpTools, err := adapter.ListTools(context.Background())
	require.NoError(t, err)
	assert.Len(t, mcpTools, 2)

	// Tools may be returned in any order, so find them by name.
	toolMap := make(map[string]int)
	for i, tool := range mcpTools {
		toolMap[tool.Name] = i
	}

	// Verify tool 1.
	idx1 := toolMap["test_tool_1"]
	assert.Equal(t, "test_tool_1", mcpTools[idx1].Name)
	assert.Equal(t, "First test tool", mcpTools[idx1].Description)
	assert.NotNil(t, mcpTools[idx1].InputSchema)

	schema1 := mcpTools[idx1].InputSchema
	assert.Equal(t, "object", schema1["type"])
	props1 := schema1["properties"].(map[string]interface{})
	assert.Contains(t, props1, "arg1")
	arg1Schema := props1["arg1"].(map[string]interface{})
	assert.Equal(t, "string", arg1Schema["type"])
	assert.Equal(t, "First argument", arg1Schema["description"])

	required1 := schema1["required"].([]string)
	assert.Contains(t, required1, "arg1")

	// Verify tool 2.
	idx2 := toolMap["test_tool_2"]
	assert.Equal(t, "test_tool_2", mcpTools[idx2].Name)
	assert.Equal(t, "Second test tool", mcpTools[idx2].Description)

	schema2 := mcpTools[idx2].InputSchema
	props2 := schema2["properties"].(map[string]interface{})
	countSchema := props2["count"].(map[string]interface{})
	assert.Equal(t, "integer", countSchema["type"])
	assert.Equal(t, 10, countSchema["default"])
}

func TestExecuteTool_Success(t *testing.T) {
	registry := tools.NewRegistry()

	tool := &mockTool{
		name:        "echo",
		description: "Echo tool",
		parameters:  []tools.Parameter{},
		executeFunc: func(ctx context.Context, args map[string]interface{}) (*tools.Result, error) {
			message := args["message"].(string)
			return &tools.Result{
				Success: true,
				Output:  "Echo: " + message,
			}, nil
		},
	}

	err := registry.Register(tool)
	require.NoError(t, err)

	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)

	result, err := adapter.ExecuteTool(context.Background(), "echo", map[string]interface{}{
		"message": "hello",
	})

	require.NoError(t, err)
	assert.False(t, result.IsError)
	require.Len(t, result.Content, 1)
	assert.Equal(t, "text", result.Content[0].Type)
	assert.Equal(t, "Echo: hello", result.Content[0].Text)
}

func TestExecuteTool_Error(t *testing.T) {
	registry := tools.NewRegistry()

	tool := &mockTool{
		name:        "fail",
		description: "Failing tool",
		parameters:  []tools.Parameter{},
		executeFunc: func(ctx context.Context, args map[string]interface{}) (*tools.Result, error) {
			return nil, errors.New("intentional failure")
		},
	}

	err := registry.Register(tool)
	require.NoError(t, err)

	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)

	result, err := adapter.ExecuteTool(context.Background(), "fail", nil)

	require.NoError(t, err) // Adapter returns error as MCP content, not Go error.
	assert.True(t, result.IsError)
	require.Len(t, result.Content, 1)
	assert.Contains(t, result.Content[0].Text, "intentional failure")
}

func TestExecuteTool_WithData(t *testing.T) {
	registry := tools.NewRegistry()

	tool := &mockTool{
		name:        "stats",
		description: "Stats tool",
		parameters:  []tools.Parameter{},
		executeFunc: func(ctx context.Context, args map[string]interface{}) (*tools.Result, error) {
			return &tools.Result{
				Success: true,
				Output:  "Statistics computed",
				Data: map[string]interface{}{
					"count": 42,
					"avg":   3.14,
				},
			}, nil
		},
	}

	err := registry.Register(tool)
	require.NoError(t, err)

	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)

	result, err := adapter.ExecuteTool(context.Background(), "stats", nil)

	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Len(t, result.Content, 3) // Output + 2 data items.

	// First content is the output.
	assert.Equal(t, "Statistics computed", result.Content[0].Text)

	// Next items are data.
	texts := []string{result.Content[1].Text, result.Content[2].Text}
	assert.Contains(t, texts, "Data 'count': 42")
	assert.Contains(t, texts, "Data 'avg': 3.14")
}

func TestGetTool(t *testing.T) {
	registry := tools.NewRegistry()

	tool := &mockTool{
		name:        "get_test",
		description: "Test get tool",
		parameters: []tools.Parameter{
			{
				Name:        "param1",
				Description: "Parameter 1",
				Type:        tools.ParamTypeString,
				Required:    true,
			},
		},
	}

	err := registry.Register(tool)
	require.NoError(t, err)

	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)

	mcpTool, err := adapter.GetTool("get_test")
	require.NoError(t, err)
	assert.Equal(t, "get_test", mcpTool.Name)
	assert.Equal(t, "Test get tool", mcpTool.Description)
	assert.NotNil(t, mcpTool.InputSchema)
}

func TestGetTool_NotFound(t *testing.T) {
	registry := tools.NewRegistry()
	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)

	_, err := adapter.GetTool("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tool not found")
}

func TestParametersToSchema(t *testing.T) {
	registry := tools.NewRegistry()
	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)

	tests := []struct {
		name   string
		params []tools.Parameter
		check  func(t *testing.T, schema map[string]interface{})
	}{
		{
			name: "string parameter",
			params: []tools.Parameter{
				{
					Name:        "name",
					Description: "Name parameter",
					Type:        tools.ParamTypeString,
					Required:    true,
				},
			},
			check: func(t *testing.T, schema map[string]interface{}) {
				assert.Equal(t, "object", schema["type"])
				props := schema["properties"].(map[string]interface{})
				nameSchema := props["name"].(map[string]interface{})
				assert.Equal(t, "string", nameSchema["type"])
				assert.Equal(t, "Name parameter", nameSchema["description"])
				required := schema["required"].([]string)
				assert.Contains(t, required, "name")
			},
		},
		{
			name: "integer parameter with default",
			params: []tools.Parameter{
				{
					Name:        "count",
					Description: "Count parameter",
					Type:        tools.ParamTypeInt,
					Required:    false,
					Default:     5,
				},
			},
			check: func(t *testing.T, schema map[string]interface{}) {
				props := schema["properties"].(map[string]interface{})
				countSchema := props["count"].(map[string]interface{})
				assert.Equal(t, "integer", countSchema["type"])
				assert.Equal(t, 5, countSchema["default"])
				_, hasRequired := schema["required"]
				assert.False(t, hasRequired, "should not have required field")
			},
		},
		{
			name: "boolean parameter",
			params: []tools.Parameter{
				{
					Name:        "enabled",
					Description: "Enabled flag",
					Type:        tools.ParamTypeBool,
					Required:    false,
				},
			},
			check: func(t *testing.T, schema map[string]interface{}) {
				props := schema["properties"].(map[string]interface{})
				enabledSchema := props["enabled"].(map[string]interface{})
				assert.Equal(t, "boolean", enabledSchema["type"])
			},
		},
		{
			name: "array parameter",
			params: []tools.Parameter{
				{
					Name:        "items",
					Description: "Items array",
					Type:        tools.ParamTypeArray,
					Required:    true,
				},
			},
			check: func(t *testing.T, schema map[string]interface{}) {
				props := schema["properties"].(map[string]interface{})
				itemsSchema := props["items"].(map[string]interface{})
				assert.Equal(t, "array", itemsSchema["type"])
			},
		},
		{
			name: "object parameter",
			params: []tools.Parameter{
				{
					Name:        "config",
					Description: "Configuration object",
					Type:        tools.ParamTypeObject,
					Required:    false,
				},
			},
			check: func(t *testing.T, schema map[string]interface{}) {
				props := schema["properties"].(map[string]interface{})
				configSchema := props["config"].(map[string]interface{})
				assert.Equal(t, "object", configSchema["type"])
			},
		},
		{
			name: "multiple parameters",
			params: []tools.Parameter{
				{
					Name:        "name",
					Description: "Name",
					Type:        tools.ParamTypeString,
					Required:    true,
				},
				{
					Name:        "age",
					Description: "Age",
					Type:        tools.ParamTypeInt,
					Required:    true,
				},
				{
					Name:        "active",
					Description: "Active status",
					Type:        tools.ParamTypeBool,
					Required:    false,
				},
			},
			check: func(t *testing.T, schema map[string]interface{}) {
				props := schema["properties"].(map[string]interface{})
				assert.Len(t, props, 3)
				required := schema["required"].([]string)
				assert.Len(t, required, 2)
				assert.Contains(t, required, "name")
				assert.Contains(t, required, "age")
				assert.NotContains(t, required, "active")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := adapter.parametersToSchema(tt.params)
			tt.check(t, schema)
		})
	}
}

func TestConvertResultToMCP_ErrorResult(t *testing.T) {
	registry := tools.NewRegistry()
	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)

	result := &tools.Result{
		Success: false,
		Output:  "Operation failed",
		Error:   errors.New("something went wrong"),
	}

	mcpResult := adapter.convertResultToMCP(result)
	assert.True(t, mcpResult.IsError)
	assert.Len(t, mcpResult.Content, 2) // Output + error.

	texts := []string{mcpResult.Content[0].Text, mcpResult.Content[1].Text}
	assert.Contains(t, texts, "Operation failed")
	assert.Contains(t, texts, "Error: something went wrong")
}

func TestListTools_EmptyRegistry(t *testing.T) {
	registry := tools.NewRegistry()
	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)

	mcpTools, err := adapter.ListTools(context.Background())
	require.NoError(t, err)
	assert.Empty(t, mcpTools)
}
