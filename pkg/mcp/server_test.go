package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/mcp/protocol"
)

func TestNewServer(t *testing.T) {
	registry := tools.NewRegistry()
	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)

	server := NewServer(adapter)
	assert.NotNil(t, server)
	assert.Equal(t, adapter, server.adapter)
	assert.NotNil(t, server.handler)
	assert.False(t, server.initialized)
	assert.Equal(t, "atmos-mcp-server", server.serverInfo.Name)
}

func TestServerInfo(t *testing.T) {
	registry := tools.NewRegistry()
	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)
	server := NewServer(adapter)

	info := server.ServerInfo()
	assert.Equal(t, "atmos-mcp-server", info.Name)
	assert.NotEmpty(t, info.Version)
}

func TestHandler(t *testing.T) {
	registry := tools.NewRegistry()
	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)
	server := NewServer(adapter)

	handler := server.Handler()
	assert.NotNil(t, handler)
}

func TestHandleInitialize(t *testing.T) {
	registry := tools.NewRegistry()
	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)
	server := NewServer(adapter)

	params := protocol.InitializeParams{
		ProtocolVersion: protocol.ProtocolVersion,
		Capabilities:    protocol.ClientCapabilities{},
		ClientInfo: protocol.Implementation{
			Name:    "test-client",
			Version: "1.0.0",
		},
	}
	paramsJSON, _ := json.Marshal(params)

	result, err := server.handleInitialize(context.Background(), paramsJSON)
	require.NoError(t, err)

	initResult, ok := result.(protocol.InitializeResult)
	require.True(t, ok)

	assert.Equal(t, protocol.ProtocolVersion, initResult.ProtocolVersion)
	assert.Equal(t, "atmos-mcp-server", initResult.ServerInfo.Name)
	assert.NotNil(t, initResult.Capabilities.Tools)
	assert.False(t, initResult.Capabilities.Tools.ListChanged)
	assert.NotEmpty(t, initResult.Instructions)

	// Server is not initialized until initialized notification is received.
	assert.False(t, server.initialized)

	// Send initialized notification.
	err = server.handleInitialized(context.Background(), nil)
	require.NoError(t, err)

	// Now server should be initialized.
	assert.True(t, server.initialized)
}

func TestHandleInitialize_InvalidParams(t *testing.T) {
	registry := tools.NewRegistry()
	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)
	server := NewServer(adapter)

	invalidJSON := []byte(`{invalid}`)

	_, err := server.handleInitialize(context.Background(), invalidJSON)
	require.Error(t, err)
}

func TestHandleToolsList(t *testing.T) {
	registry := tools.NewRegistry()

	// Register a test tool.
	tool := &mockTool{
		name:        "test_tool",
		description: "Test tool",
		parameters: []tools.Parameter{
			{
				Name:        "arg1",
				Description: "Argument 1",
				Type:        tools.ParamTypeString,
				Required:    true,
			},
		},
	}
	err := registry.Register(tool)
	require.NoError(t, err)

	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)
	server := NewServer(adapter)

	// Initialize server first.
	err = server.handleInitialized(context.Background(), nil)
	require.NoError(t, err)

	result, err := server.handleToolsList(context.Background(), nil)
	require.NoError(t, err)

	listResult, ok := result.(protocol.ToolsListResult)
	require.True(t, ok)

	require.Len(t, listResult.Tools, 1)
	assert.Equal(t, "test_tool", listResult.Tools[0].Name)
	assert.Equal(t, "Test tool", listResult.Tools[0].Description)
}

func TestHandleToolsCall(t *testing.T) {
	registry := tools.NewRegistry()

	// Register a test tool.
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
	server := NewServer(adapter)

	// Initialize server first.
	err = server.handleInitialized(context.Background(), nil)
	require.NoError(t, err)

	callParams := protocol.CallToolParams{
		Name: "echo",
		Arguments: map[string]interface{}{
			"message": "hello world",
		},
	}
	paramsJSON, _ := json.Marshal(callParams)

	result, err := server.handleToolsCall(context.Background(), paramsJSON)
	require.NoError(t, err)

	callResult, ok := result.(*protocol.CallToolResult)
	require.True(t, ok)
	assert.False(t, callResult.IsError)
	require.Len(t, callResult.Content, 1)
	assert.Equal(t, "Echo: hello world", callResult.Content[0].Text)
}

func TestHandleToolsCall_InvalidParams(t *testing.T) {
	registry := tools.NewRegistry()
	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)
	server := NewServer(adapter)

	invalidJSON := []byte(`{invalid}`)

	_, err := server.handleToolsCall(context.Background(), invalidJSON)
	require.Error(t, err)
}

func TestHandlePing(t *testing.T) {
	registry := tools.NewRegistry()
	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)
	server := NewServer(adapter)

	result, err := server.handlePing(context.Background(), nil)
	require.NoError(t, err)

	pingResult, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "pong", pingResult["status"])
}

func TestHandleInitialized(t *testing.T) {
	registry := tools.NewRegistry()
	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)
	server := NewServer(adapter)

	err := server.handleInitialized(context.Background(), nil)
	require.NoError(t, err)
}

func TestServerIntegration_FullWorkflow(t *testing.T) {
	// Create a complete server setup.
	registry := tools.NewRegistry()

	tool := &mockTool{
		name:        "greet",
		description: "Greeting tool",
		parameters: []tools.Parameter{
			{
				Name:        "name",
				Description: "Name to greet",
				Type:        tools.ParamTypeString,
				Required:    true,
			},
		},
		executeFunc: func(ctx context.Context, args map[string]interface{}) (*tools.Result, error) {
			name := args["name"].(string)
			return &tools.Result{
				Success: true,
				Output:  "Hello, " + name + "!",
			}, nil
		},
	}
	err := registry.Register(tool)
	require.NoError(t, err)

	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)
	server := NewServer(adapter)

	ctx := context.Background()

	// Step 1: Initialize.
	initParams := protocol.InitializeParams{
		ProtocolVersion: protocol.ProtocolVersion,
		Capabilities:    protocol.ClientCapabilities{},
		ClientInfo: protocol.Implementation{
			Name:    "test-client",
			Version: "1.0.0",
		},
	}
	initParamsJSON, _ := json.Marshal(initParams)

	initReq := &protocol.Request{
		JSONRPC: protocol.JSONRPCVersion,
		ID:      1,
		Method:  protocol.MethodInitialize,
		Params:  initParamsJSON,
	}

	initResp := server.Handler().HandleRequest(ctx, initReq)
	assert.Nil(t, initResp.Error)
	assert.NotNil(t, initResp.Result)

	// Step 2: Send initialized notification.
	initNotif := &protocol.Notification{
		JSONRPC: protocol.JSONRPCVersion,
		Method:  protocol.MethodInitialized,
	}

	err = server.Handler().HandleNotification(ctx, initNotif)
	assert.NoError(t, err)

	// Step 3: List tools.
	listReq := &protocol.Request{
		JSONRPC: protocol.JSONRPCVersion,
		ID:      2,
		Method:  protocol.MethodToolsList,
	}

	listResp := server.Handler().HandleRequest(ctx, listReq)
	assert.Nil(t, listResp.Error)
	listResult := listResp.Result.(protocol.ToolsListResult)
	assert.Len(t, listResult.Tools, 1)
	assert.Equal(t, "greet", listResult.Tools[0].Name)

	// Step 4: Call tool.
	callParams := protocol.CallToolParams{
		Name: "greet",
		Arguments: map[string]interface{}{
			"name": "Alice",
		},
	}
	callParamsJSON, _ := json.Marshal(callParams)

	callReq := &protocol.Request{
		JSONRPC: protocol.JSONRPCVersion,
		ID:      3,
		Method:  protocol.MethodToolsCall,
		Params:  callParamsJSON,
	}

	callResp := server.Handler().HandleRequest(ctx, callReq)
	assert.Nil(t, callResp.Error)
	callResult := callResp.Result.(*protocol.CallToolResult)
	assert.False(t, callResult.IsError)
	assert.Equal(t, "Hello, Alice!", callResult.Content[0].Text)

	// Step 5: Ping.
	pingReq := &protocol.Request{
		JSONRPC: protocol.JSONRPCVersion,
		ID:      4,
		Method:  protocol.MethodPing,
	}

	pingResp := server.Handler().HandleRequest(ctx, pingReq)
	assert.Nil(t, pingResp.Error)
	pingResult := pingResp.Result.(map[string]interface{})
	assert.Equal(t, "pong", pingResult["status"])
}

func TestServerWithMultipleTools(t *testing.T) {
	registry := tools.NewRegistry()

	// Register multiple tools.
	for i := 1; i <= 5; i++ {
		tool := &mockTool{
			name:        "tool_" + string(rune('0'+i)),
			description: "Tool " + string(rune('0'+i)),
			parameters:  []tools.Parameter{},
		}
		err := registry.Register(tool)
		require.NoError(t, err)
	}

	executor := tools.NewExecutor(registry, nil, tools.DefaultTimeout)
	adapter := NewAdapter(registry, executor)
	server := NewServer(adapter)

	// Initialize server first.
	err := server.handleInitialized(context.Background(), nil)
	require.NoError(t, err)

	result, err := server.handleToolsList(context.Background(), nil)
	require.NoError(t, err)

	listResult := result.(protocol.ToolsListResult)
	assert.Len(t, listResult.Tools, 5)
}
