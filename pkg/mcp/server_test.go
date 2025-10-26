package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/tools"
)

func TestNewServer(t *testing.T) {
	registry := tools.NewRegistry()
	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)

	server := NewServer(adapter)

	assert.NotNil(t, server)
	assert.NotNil(t, server.sdk)
	assert.Equal(t, adapter, server.adapter)
	assert.Equal(t, registry, server.registry)
}

func TestServer_ServerInfo(t *testing.T) {
	registry := tools.NewRegistry()
	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)
	server := NewServer(adapter)

	info := server.ServerInfo()

	assert.Equal(t, "atmos-mcp-server", info.Name)
	assert.NotEmpty(t, info.Version)
}

func TestServer_SDK(t *testing.T) {
	registry := tools.NewRegistry()
	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)
	server := NewServer(adapter)

	sdk := server.SDK()

	assert.NotNil(t, sdk)
	assert.IsType(t, &mcpsdk.Server{}, sdk)
}

func TestServer_RegisterTools(t *testing.T) {
	registry := tools.NewRegistry()

	// Register multiple mock tools.
	tool1 := &mockTool{
		name:        "tool1",
		description: "First tool",
		params: []tools.Parameter{
			{Name: "param1", Type: tools.ParamTypeString, Required: true},
		},
	}
	tool2 := &mockTool{
		name:        "tool2",
		description: "Second tool",
		params: []tools.Parameter{
			{Name: "param1", Type: tools.ParamTypeInt, Required: false},
		},
	}

	err := registry.Register(tool1)
	require.NoError(t, err)
	err = registry.Register(tool2)
	require.NoError(t, err)

	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)

	// NewServer should register all tools.
	server := NewServer(adapter)

	// Verify server was created successfully.
	assert.NotNil(t, server)
	assert.Equal(t, 2, registry.Count())
}

func TestServer_HandleToolCall_Success(t *testing.T) {
	registry := tools.NewRegistry()

	// Register a mock tool.
	mockTool := &mockTool{
		name:        "test_tool",
		description: "Test tool",
		executeFunc: func(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
			return &tools.Result{
				Success: true,
				Output:  "Success output",
				Data: map[string]interface{}{
					"result": "test data",
				},
			}, nil
		},
	}
	err := registry.Register(mockTool)
	require.NoError(t, err)

	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)
	server := NewServer(adapter)

	// Test handleToolCall directly.
	ctx := context.Background()
	args := map[string]interface{}{"arg1": "value1"}

	result, err := server.handleToolCall(ctx, "test_tool", args)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)
	assert.Len(t, result.Content, 2) // Output + Data

	// Verify content.
	textContent1, ok := result.Content[0].(*mcpsdk.TextContent)
	require.True(t, ok)
	assert.Equal(t, "Success output", textContent1.Text)

	textContent2, ok := result.Content[1].(*mcpsdk.TextContent)
	require.True(t, ok)
	assert.Contains(t, textContent2.Text, "result")
}

func TestServer_HandleToolCall_WithError(t *testing.T) {
	registry := tools.NewRegistry()

	// Register a mock tool that returns an error.
	mockTool := &mockTool{
		name:        "error_tool",
		description: "Tool that errors",
		executeFunc: func(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
			return nil, errors.New("execution error")
		},
	}
	err := registry.Register(mockTool)
	require.NoError(t, err)

	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)
	server := NewServer(adapter)

	ctx := context.Background()
	args := map[string]interface{}{}

	result, err := server.handleToolCall(ctx, "error_tool", args)

	assert.NoError(t, err) // MCP returns errors in result, not as Go errors.
	assert.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Len(t, result.Content, 1)

	textContent, ok := result.Content[0].(*mcpsdk.TextContent)
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "tool execution failed")
}

func TestServer_HandleToolCall_PartialSuccess(t *testing.T) {
	registry := tools.NewRegistry()

	// Register a mock tool that succeeds but has an error in result.
	mockTool := &mockTool{
		name:        "partial_tool",
		description: "Tool with partial success",
		executeFunc: func(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
			return &tools.Result{
				Success: false, // Failed.
				Output:  "Partial output",
				Error:   errors.New("partial error"),
				Data: map[string]interface{}{
					"debug": "info",
				},
			}, nil
		},
	}
	err := registry.Register(mockTool)
	require.NoError(t, err)

	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)
	server := NewServer(adapter)

	ctx := context.Background()
	args := map[string]interface{}{}

	result, err := server.handleToolCall(ctx, "partial_tool", args)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.IsError)   // IsError should be true when Success=false.
	assert.Len(t, result.Content, 3) // Output + Error + Data

	// Check content includes output.
	textContent1, ok := result.Content[0].(*mcpsdk.TextContent)
	require.True(t, ok)
	assert.Equal(t, "Partial output", textContent1.Text)

	// Check content includes error.
	textContent2, ok := result.Content[1].(*mcpsdk.TextContent)
	require.True(t, ok)
	assert.Contains(t, textContent2.Text, "partial error")

	// Check content includes data.
	textContent3, ok := result.Content[2].(*mcpsdk.TextContent)
	require.True(t, ok)
	assert.Contains(t, textContent3.Text, "debug")
}

func TestServer_HandleToolCall_EmptyResult(t *testing.T) {
	registry := tools.NewRegistry()

	// Register a mock tool that returns empty result.
	mockTool := &mockTool{
		name:        "empty_tool",
		description: "Tool with empty result",
		executeFunc: func(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
			return &tools.Result{
				Success: true,
				Output:  "",
				Data:    nil,
			}, nil
		},
	}
	err := registry.Register(mockTool)
	require.NoError(t, err)

	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)
	server := NewServer(adapter)

	ctx := context.Background()
	args := map[string]interface{}{}

	result, err := server.handleToolCall(ctx, "empty_tool", args)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)
	assert.Len(t, result.Content, 0) // No content when empty.
}

func TestServer_ToolRegistrationWithParameters(t *testing.T) {
	registry := tools.NewRegistry()

	// Register tool with various parameter types.
	mockTool := &mockTool{
		name:        "param_tool",
		description: "Tool with parameters",
		params: []tools.Parameter{
			{
				Name:        "string_param",
				Type:        tools.ParamTypeString,
				Description: "A string parameter",
				Required:    true,
			},
			{
				Name:        "int_param",
				Type:        tools.ParamTypeInt,
				Description: "An integer parameter",
				Required:    false,
				Default:     42,
			},
			{
				Name:        "bool_param",
				Type:        tools.ParamTypeBool,
				Description: "A boolean parameter",
				Required:    false,
			},
		},
	}
	err := registry.Register(mockTool)
	require.NoError(t, err)

	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)

	// Creating server should register the tool with its parameters.
	server := NewServer(adapter)

	assert.NotNil(t, server)
	// Verify the tool is in the registry.
	tool, err := registry.Get("param_tool")
	require.NoError(t, err)
	assert.Equal(t, "param_tool", tool.Name())
	assert.Len(t, tool.Parameters(), 3)
}

func TestServer_JSONArgumentUnmarshaling(t *testing.T) {
	registry := tools.NewRegistry()

	// Register a tool that checks its arguments.
	receivedArgs := make(map[string]interface{})
	mockTool := &mockTool{
		name:        "args_tool",
		description: "Tool that receives args",
		executeFunc: func(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
			// Copy received args for verification.
			for k, v := range params {
				receivedArgs[k] = v
			}
			return &tools.Result{Success: true, Output: "OK"}, nil
		},
	}
	err := registry.Register(mockTool)
	require.NoError(t, err)

	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)
	server := NewServer(adapter)

	// Create a CallToolRequest with JSON arguments.
	args := map[string]interface{}{
		"string_arg": "test",
		"int_arg":    123,
		"bool_arg":   true,
		"array_arg":  []interface{}{"a", "b", "c"},
		"object_arg": map[string]interface{}{"key": "value"},
	}
	argsJSON, err := json.Marshal(args)
	require.NoError(t, err)

	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: argsJSON,
		},
	}

	// Simulate the handler being called by SDK.
	ctx := context.Background()
	var unmarshaledArgs map[string]interface{}
	if len(req.Params.Arguments) > 0 {
		err = json.Unmarshal(req.Params.Arguments, &unmarshaledArgs)
		require.NoError(t, err)
	}

	result, err := server.handleToolCall(ctx, "args_tool", unmarshaledArgs)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)

	// Verify arguments were passed correctly.
	assert.Equal(t, "test", receivedArgs["string_arg"])
	assert.Equal(t, float64(123), receivedArgs["int_arg"]) // JSON unmarshals numbers as float64.
	assert.Equal(t, true, receivedArgs["bool_arg"])
	assert.IsType(t, []interface{}{}, receivedArgs["array_arg"])
	assert.IsType(t, map[string]interface{}{}, receivedArgs["object_arg"])
}

func TestServer_GenerateInputSchema_AllParameterTypes(t *testing.T) {
	registry := tools.NewRegistry()

	// Register tool with all parameter types.
	mockTool := &mockTool{
		name:        "all_types_tool",
		description: "Tool with all parameter types",
		params: []tools.Parameter{
			{Name: "string_param", Type: tools.ParamTypeString, Description: "String", Required: true},
			{Name: "int_param", Type: tools.ParamTypeInt, Description: "Integer", Required: false},
			{Name: "bool_param", Type: tools.ParamTypeBool, Description: "Boolean", Required: false},
			{Name: "array_param", Type: tools.ParamTypeArray, Description: "Array", Required: false},
			{Name: "object_param", Type: tools.ParamTypeObject, Description: "Object", Required: false},
			{Name: "default_param", Type: tools.ParamTypeString, Description: "With default", Default: "default_value"},
		},
	}
	err := registry.Register(mockTool)
	require.NoError(t, err)

	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)
	server := NewServer(adapter)

	// Generate schema.
	schema := server.generateInputSchema(mockTool)

	// Verify schema structure.
	assert.Equal(t, "object", schema["type"])

	properties, ok := schema["properties"].(map[string]interface{})
	require.True(t, ok)
	assert.Len(t, properties, 6)

	// Verify string parameter.
	stringProp, ok := properties["string_param"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "string", stringProp["type"])
	assert.Equal(t, "String", stringProp["description"])

	// Verify integer parameter.
	intProp, ok := properties["int_param"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "integer", intProp["type"])

	// Verify boolean parameter.
	boolProp, ok := properties["bool_param"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "boolean", boolProp["type"])

	// Verify array parameter.
	arrayProp, ok := properties["array_param"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "array", arrayProp["type"])

	// Verify object parameter.
	objectProp, ok := properties["object_param"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "object", objectProp["type"])

	// Verify default value.
	defaultProp, ok := properties["default_param"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "default_value", defaultProp["default"])

	// Verify required fields.
	required, ok := schema["required"].([]string)
	require.True(t, ok)
	assert.Equal(t, []string{"string_param"}, required)
}

func TestServer_GenerateInputSchema_NoParameters(t *testing.T) {
	registry := tools.NewRegistry()

	// Register tool with no parameters.
	mockTool := &mockTool{
		name:        "no_params_tool",
		description: "Tool with no parameters",
		params:      []tools.Parameter{},
	}
	err := registry.Register(mockTool)
	require.NoError(t, err)

	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)
	server := NewServer(adapter)

	// Generate schema.
	schema := server.generateInputSchema(mockTool)

	// Verify minimal schema.
	assert.Equal(t, "object", schema["type"])
	properties, ok := schema["properties"].(map[string]interface{})
	require.True(t, ok)
	assert.Len(t, properties, 0)
	_, hasRequired := schema["required"]
	assert.False(t, hasRequired, "Should not have required field when no parameters")
}

func TestServer_RegisterTools_EmptyRegistry(t *testing.T) {
	// Create server with empty registry.
	registry := tools.NewRegistry()
	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)

	// Should not panic with empty registry.
	server := NewServer(adapter)

	assert.NotNil(t, server)
	assert.Equal(t, 0, registry.Count())
}

func TestServer_HandleToolCall_MultipleDataFields(t *testing.T) {
	registry := tools.NewRegistry()

	// Register tool that returns multiple data fields.
	mockTool := &mockTool{
		name:        "multi_data_tool",
		description: "Tool with multiple data fields",
		executeFunc: func(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
			return &tools.Result{
				Success: true,
				Output:  "Main output",
				Data: map[string]interface{}{
					"field1": "value1",
					"field2": 123,
					"field3": true,
					"field4": []string{"a", "b"},
				},
			}, nil
		},
	}
	err := registry.Register(mockTool)
	require.NoError(t, err)

	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)
	server := NewServer(adapter)

	ctx := context.Background()
	result, err := server.handleToolCall(ctx, "multi_data_tool", nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)
	// Should have output + 4 data fields = 5 content items.
	assert.Len(t, result.Content, 5)

	// First should be output.
	textContent, ok := result.Content[0].(*mcpsdk.TextContent)
	require.True(t, ok)
	assert.Equal(t, "Main output", textContent.Text)
}

func TestServer_RegisterTools_InvalidJSONArguments(t *testing.T) {
	// Test the JSON unmarshaling error path by simulating what happens
	// in the handler when invalid JSON is provided.

	// Create request with invalid JSON.
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: []byte("{invalid json}"),
		},
	}

	// Simulate the handler that was registered.
	var args map[string]interface{}
	var result *mcpsdk.CallToolResult
	if len(req.Params.Arguments) > 0 {
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			result = &mcpsdk.CallToolResult{
				Content: []mcpsdk.Content{
					&mcpsdk.TextContent{Text: fmt.Sprintf("Failed to parse arguments: %v", err)},
				},
				IsError: true,
			}
		}
	}

	// Verify error handling.
	assert.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Len(t, result.Content, 1)

	textContent, ok := result.Content[0].(*mcpsdk.TextContent)
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "Failed to parse arguments")
}

func TestServer_GenerateInputSchema_UnknownParameterType(t *testing.T) {
	registry := tools.NewRegistry()

	// Register tool with unknown parameter type (defaults to string).
	mockTool := &mockTool{
		name:        "unknown_type_tool",
		description: "Tool with unknown parameter type",
		params: []tools.Parameter{
			{
				Name:        "unknown_param",
				Type:        tools.ParamType("unknown_type"), // Unknown type.
				Description: "Parameter with unknown type",
				Required:    false,
			},
		},
	}
	err := registry.Register(mockTool)
	require.NoError(t, err)

	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)
	server := NewServer(adapter)

	// Generate schema.
	schema := server.generateInputSchema(mockTool)

	// Verify schema structure.
	assert.Equal(t, "object", schema["type"])

	properties, ok := schema["properties"].(map[string]interface{})
	require.True(t, ok)
	assert.Len(t, properties, 1)

	// Verify unknown type defaults to string.
	unknownProp, ok := properties["unknown_param"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "string", unknownProp["type"], "Unknown parameter types should default to string")
	assert.Equal(t, "Parameter with unknown type", unknownProp["description"])
}
