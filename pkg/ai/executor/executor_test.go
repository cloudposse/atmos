package executor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/formatter"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// mockClient is a mock AI client for testing.
type mockClient struct {
	responses            []*types.Response
	callIndex            int
	sendMessageError     error
	sendMessageResponse  string
	sendMessageWithError error
}

func (m *mockClient) SendMessage(ctx context.Context, message string) (string, error) {
	if m.sendMessageError != nil {
		return "", m.sendMessageError
	}
	if m.sendMessageResponse != "" {
		return m.sendMessageResponse, nil
	}
	return "Mock response", nil
}

func (m *mockClient) SendMessageWithTools(ctx context.Context, message string, availableTools []tools.Tool) (*types.Response, error) {
	if m.sendMessageWithError != nil {
		return nil, m.sendMessageWithError
	}
	if m.callIndex >= len(m.responses) {
		return &types.Response{
			Content:    "Final response",
			StopReason: types.StopReasonEndTurn,
		}, nil
	}

	response := m.responses[m.callIndex]
	m.callIndex++
	return response, nil
}

func (m *mockClient) SendMessageWithHistory(ctx context.Context, messages []types.Message) (string, error) {
	return "Mock response with history", nil
}

func (m *mockClient) SendMessageWithToolsAndHistory(ctx context.Context, messages []types.Message, availableTools []tools.Tool) (*types.Response, error) {
	if m.sendMessageWithError != nil {
		return nil, m.sendMessageWithError
	}
	if m.callIndex >= len(m.responses) {
		return &types.Response{
			Content:    "Final response",
			StopReason: types.StopReasonEndTurn,
		}, nil
	}

	response := m.responses[m.callIndex]
	m.callIndex++
	return response, nil
}

func (m *mockClient) SendMessageWithSystemPromptAndTools(ctx context.Context, systemPrompt string, atmosMemory string, messages []types.Message, availableTools []tools.Tool) (*types.Response, error) {
	if m.sendMessageWithError != nil {
		return nil, m.sendMessageWithError
	}
	if m.callIndex >= len(m.responses) {
		return &types.Response{
			Content:    "Final response",
			StopReason: types.StopReasonEndTurn,
		}, nil
	}

	response := m.responses[m.callIndex]
	m.callIndex++
	return response, nil
}

func (m *mockClient) GetModel() string {
	return "mock-model"
}

func (m *mockClient) GetMaxTokens() int {
	return 4096
}

// mockTool is a mock tool for testing.
type mockTool struct {
	name        string
	description string
	params      []tools.Parameter
	executeFunc func(ctx context.Context, params map[string]interface{}) (*tools.Result, error)
}

func (m *mockTool) Name() string {
	return m.name
}

func (m *mockTool) Description() string {
	return m.description
}

func (m *mockTool) Parameters() []tools.Parameter {
	return m.params
}

func (m *mockTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, params)
	}
	return &tools.Result{Success: true, Output: "mock output"}, nil
}

func (m *mockTool) RequiresPermission() bool {
	return false
}

func (m *mockTool) IsRestricted() bool {
	return false
}

// mockPermissionChecker is a mock permission checker that always allows execution.
type mockPermissionChecker struct{}

func (m *mockPermissionChecker) CheckPermission(ctx context.Context, tool interface{}, params map[string]interface{}) (bool, error) {
	return true, nil
}

func TestNewExecutor(t *testing.T) {
	tests := []struct {
		name         string
		client       *mockClient
		toolExecutor *tools.Executor
		atmosConfig  *schema.AtmosConfiguration
	}{
		{
			name:         "creates executor with all params",
			client:       &mockClient{},
			toolExecutor: nil,
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						DefaultProvider: "mock",
					},
				},
			},
		},
		{
			name:         "creates executor with nil tool executor",
			client:       &mockClient{},
			toolExecutor: nil,
			atmosConfig:  nil,
		},
		{
			name:         "creates executor with nil config",
			client:       &mockClient{},
			toolExecutor: nil,
			atmosConfig:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewExecutor(tt.client, tt.toolExecutor, tt.atmosConfig)

			assert.NotNil(t, exec)
			assert.Equal(t, tt.client, exec.client)
			assert.Equal(t, tt.toolExecutor, exec.toolExecutor)
			assert.Equal(t, tt.atmosConfig, exec.atmosConfig)
		})
	}
}

func TestExecutor_ExecuteSimple(t *testing.T) {
	tests := []struct {
		name           string
		client         *mockClient
		atmosConfig    *schema.AtmosConfiguration
		opts           Options
		expectedResult *formatter.ExecutionResult
	}{
		{
			name:   "successful simple execution",
			client: &mockClient{sendMessageResponse: "Test response"},
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						DefaultProvider: "mock",
					},
				},
			},
			opts: Options{
				Prompt:       "What is Atmos?",
				ToolsEnabled: false,
			},
			expectedResult: &formatter.ExecutionResult{
				Success:  true,
				Response: "Test response",
			},
		},
		{
			name:   "execution with session ID",
			client: &mockClient{sendMessageResponse: "Response with session"},
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						DefaultProvider: "mock",
					},
				},
			},
			opts: Options{
				Prompt:       "Test prompt",
				ToolsEnabled: false,
				SessionID:    "test-session-123",
			},
			expectedResult: &formatter.ExecutionResult{
				Success:  true,
				Response: "Response with session",
			},
		},
		{
			name:        "execution with nil config",
			client:      &mockClient{sendMessageResponse: "Response with nil config"},
			atmosConfig: nil,
			opts: Options{
				Prompt:       "Test prompt",
				ToolsEnabled: false,
			},
			expectedResult: &formatter.ExecutionResult{
				Success:  true,
				Response: "Response with nil config",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewExecutor(tt.client, nil, tt.atmosConfig)

			result := exec.Execute(context.Background(), tt.opts)

			assert.Equal(t, tt.expectedResult.Success, result.Success)
			assert.Equal(t, tt.expectedResult.Response, result.Response)
			assert.Equal(t, "mock-model", result.Metadata.Model)
			assert.False(t, result.Metadata.ToolsEnabled)
			assert.True(t, result.Metadata.DurationMs >= 0)
		})
	}
}

func TestExecutor_ExecuteSimpleError(t *testing.T) {
	client := &mockClient{
		sendMessageError: errors.New("AI service unavailable"),
	}
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				DefaultProvider: "mock",
			},
		},
	}

	exec := NewExecutor(client, nil, atmosConfig)

	result := exec.Execute(context.Background(), Options{
		Prompt:       "Test prompt",
		ToolsEnabled: false,
	})

	assert.False(t, result.Success)
	assert.NotNil(t, result.Error)
	assert.Equal(t, "AI service unavailable", result.Error.Message)
	assert.Equal(t, "ai_error", result.Error.Type)
}

func TestExecutor_ExecuteWithToolsNoTools(t *testing.T) {
	// Test that when tools are enabled but nil toolExecutor, it panics (current behavior).
	// The executor expects a valid toolExecutor when tools are enabled.
	// This test documents the current behavior.
	client := &mockClient{sendMessageResponse: "Fallback response"}
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				DefaultProvider: "mock",
			},
		},
	}

	exec := NewExecutor(client, nil, atmosConfig)

	// When toolExecutor is nil and ToolsEnabled is true, the code panics.
	// Test this with a deferred recover.
	assert.Panics(t, func() {
		exec.Execute(context.Background(), Options{
			Prompt:       "Test prompt with tools",
			ToolsEnabled: true,
		})
	}, "Expected panic when toolExecutor is nil and tools are enabled")
}

func TestExecutor_ExecuteWithToolsEmptyRegistry(t *testing.T) {
	// Test with an empty tool registry.
	client := &mockClient{sendMessageResponse: "Fallback response"}
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				DefaultProvider: "mock",
			},
		},
	}

	// Create empty registry and executor.
	registry := tools.NewRegistry()
	toolExecutor := tools.NewExecutor(registry, nil, 30*time.Second)
	exec := NewExecutor(client, toolExecutor, atmosConfig)

	result := exec.Execute(context.Background(), Options{
		Prompt:       "Test prompt with tools",
		ToolsEnabled: true,
	})

	// Should fall back to simple execution because no tools are registered.
	assert.True(t, result.Success)
	assert.Equal(t, "Fallback response", result.Response)
}

func TestExecutor_GetProviderName(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		expected    string
	}{
		{
			name: "returns configured provider",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						DefaultProvider: "anthropic",
					},
				},
			},
			expected: "anthropic",
		},
		{
			name: "returns unknown for empty provider",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						DefaultProvider: "",
					},
				},
			},
			expected: "unknown",
		},
		{
			name:        "returns unknown for nil config",
			atmosConfig: nil,
			expected:    "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewExecutor(&mockClient{}, nil, tt.atmosConfig)

			result := exec.getProviderName()

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCombineUsage(t *testing.T) {
	tests := []struct {
		name     string
		a        *types.Usage
		b        *types.Usage
		expected *types.Usage
	}{
		{
			name:     "both nil",
			a:        nil,
			b:        nil,
			expected: nil,
		},
		{
			name: "a nil",
			a:    nil,
			b: &types.Usage{
				InputTokens:  100,
				OutputTokens: 50,
				TotalTokens:  150,
			},
			expected: &types.Usage{
				InputTokens:  100,
				OutputTokens: 50,
				TotalTokens:  150,
			},
		},
		{
			name: "b nil",
			a: &types.Usage{
				InputTokens:  100,
				OutputTokens: 50,
				TotalTokens:  150,
			},
			b: nil,
			expected: &types.Usage{
				InputTokens:  100,
				OutputTokens: 50,
				TotalTokens:  150,
			},
		},
		{
			name: "both have values",
			a: &types.Usage{
				InputTokens:         100,
				OutputTokens:        50,
				TotalTokens:         150,
				CacheReadTokens:     20,
				CacheCreationTokens: 10,
			},
			b: &types.Usage{
				InputTokens:         200,
				OutputTokens:        100,
				TotalTokens:         300,
				CacheReadTokens:     40,
				CacheCreationTokens: 20,
			},
			expected: &types.Usage{
				InputTokens:         300,
				OutputTokens:        150,
				TotalTokens:         450,
				CacheReadTokens:     60,
				CacheCreationTokens: 30,
			},
		},
		{
			name: "zero values",
			a: &types.Usage{
				InputTokens:  0,
				OutputTokens: 0,
				TotalTokens:  0,
			},
			b: &types.Usage{
				InputTokens:  0,
				OutputTokens: 0,
				TotalTokens:  0,
			},
			expected: &types.Usage{
				InputTokens:  0,
				OutputTokens: 0,
				TotalTokens:  0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := combineUsage(tt.a, tt.b)

			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.expected.InputTokens, result.InputTokens)
				assert.Equal(t, tt.expected.OutputTokens, result.OutputTokens)
				assert.Equal(t, tt.expected.TotalTokens, result.TotalTokens)
				assert.Equal(t, tt.expected.CacheReadTokens, result.CacheReadTokens)
				assert.Equal(t, tt.expected.CacheCreationTokens, result.CacheCreationTokens)
			}
		})
	}
}

func TestFormatToolResults(t *testing.T) {
	tests := []struct {
		name     string
		results  []formatter.ToolCallResult
		contains []string
	}{
		{
			name:     "empty results",
			results:  []formatter.ToolCallResult{},
			contains: []string{},
		},
		{
			name: "single successful result",
			results: []formatter.ToolCallResult{
				{
					Tool:    "test_tool",
					Success: true,
					Result:  map[string]interface{}{"output": "test"},
				},
			},
			contains: []string{"Tool 1: test_tool", "Success", "output"},
		},
		{
			name: "single failed result",
			results: []formatter.ToolCallResult{
				{
					Tool:    "test_tool",
					Success: false,
					Error:   "execution failed",
				},
			},
			contains: []string{"Tool 1: test_tool", "Failed", "execution failed"},
		},
		{
			name: "multiple results mixed success",
			results: []formatter.ToolCallResult{
				{
					Tool:    "tool_a",
					Success: true,
					Result:  "result a",
				},
				{
					Tool:    "tool_b",
					Success: false,
					Error:   "tool b error",
				},
				{
					Tool:    "tool_c",
					Success: true,
					Result:  "result c",
				},
			},
			contains: []string{
				"Tool 1: tool_a", "Tool 2: tool_b", "Tool 3: tool_c",
				"result a", "tool b error", "result c",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatToolResults(tt.results)

			for _, expected := range tt.contains {
				assert.Contains(t, result, expected)
			}
		})
	}
}

func TestExecutor_ExecuteMetadata(t *testing.T) {
	client := &mockClient{sendMessageResponse: "Test response"}
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				DefaultProvider: "test-provider",
			},
		},
	}

	exec := NewExecutor(client, nil, atmosConfig)

	beforeExec := time.Now()
	result := exec.Execute(context.Background(), Options{
		Prompt:       "Test prompt",
		ToolsEnabled: false,
		SessionID:    "session-123",
	})
	afterExec := time.Now()

	assert.Equal(t, "mock-model", result.Metadata.Model)
	assert.Equal(t, "test-provider", result.Metadata.Provider)
	assert.Equal(t, "session-123", result.Metadata.SessionID)
	assert.False(t, result.Metadata.ToolsEnabled)
	assert.True(t, result.Metadata.DurationMs >= 0)
	assert.True(t, result.Metadata.Timestamp.After(beforeExec) || result.Metadata.Timestamp.Equal(beforeExec))
	assert.True(t, result.Metadata.Timestamp.Before(afterExec) || result.Metadata.Timestamp.Equal(afterExec))
}

func TestMaxToolIterationsConstant(t *testing.T) {
	// Verify the constant is set to a reasonable value.
	assert.Equal(t, 10, MaxToolIterations)
}

func TestExecutor_ExecuteToolsEnabled(t *testing.T) {
	// Test that tools enabled flag is correctly set in metadata.
	// Note: When tools are enabled, a valid tool registry is required.
	t.Run("tools enabled false", func(t *testing.T) {
		client := &mockClient{sendMessageResponse: "Test response"}
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					DefaultProvider: "mock",
				},
			},
		}

		exec := NewExecutor(client, nil, atmosConfig)

		result := exec.Execute(context.Background(), Options{
			Prompt:       "Test prompt",
			ToolsEnabled: false,
		})

		assert.False(t, result.Metadata.ToolsEnabled)
	})

	t.Run("tools enabled true with registry", func(t *testing.T) {
		client := &mockClient{
			responses: []*types.Response{
				{
					Content:    "Response",
					StopReason: types.StopReasonEndTurn,
				},
			},
		}
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					DefaultProvider: "mock",
				},
			},
		}

		// Create registry with a tool.
		registry := tools.NewRegistry()
		mockToolInstance := &mockTool{
			name:        "test_tool",
			description: "A test tool",
		}
		err := registry.Register(mockToolInstance)
		require.NoError(t, err)

		toolExecutor := tools.NewExecutor(registry, nil, 30*time.Second)
		exec := NewExecutor(client, toolExecutor, atmosConfig)

		result := exec.Execute(context.Background(), Options{
			Prompt:       "Test prompt",
			ToolsEnabled: true,
		})

		assert.True(t, result.Metadata.ToolsEnabled)
	})
}

func TestOptions_Fields(t *testing.T) {
	opts := Options{
		Prompt:         "Test prompt",
		ToolsEnabled:   true,
		SessionID:      "session-abc",
		IncludeContext: true,
	}

	assert.Equal(t, "Test prompt", opts.Prompt)
	assert.True(t, opts.ToolsEnabled)
	assert.Equal(t, "session-abc", opts.SessionID)
	assert.True(t, opts.IncludeContext)
}

func TestExecutor_ContextCancellation(t *testing.T) {
	client := &mockClient{sendMessageResponse: "Response"}
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				DefaultProvider: "mock",
			},
		},
	}

	exec := NewExecutor(client, nil, atmosConfig)

	// Test that cancelled context is passed through.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	result := exec.Execute(ctx, Options{
		Prompt:       "Test prompt",
		ToolsEnabled: false,
	})

	// The mock client doesn't check context, so it should still succeed.
	// This test verifies that the executor doesn't panic with cancelled context.
	assert.True(t, result.Success)
}

func TestExecutor_ExecuteWithToolsError(t *testing.T) {
	// Create a client that returns an error when calling SendMessageWithSystemPromptAndTools.
	client := &mockClient{
		sendMessageWithError: errors.New("API error"),
	}
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				DefaultProvider: "mock",
			},
		},
	}

	// Create registry with a tool.
	registry := tools.NewRegistry()
	mockToolInstance := &mockTool{
		name:        "test_tool",
		description: "A test tool",
	}
	err := registry.Register(mockToolInstance)
	require.NoError(t, err)

	toolExecutor := tools.NewExecutor(registry, nil, 30*time.Second)
	exec := NewExecutor(client, toolExecutor, atmosConfig)

	result := exec.Execute(context.Background(), Options{
		Prompt:       "Test prompt with tools",
		ToolsEnabled: true,
	})

	assert.False(t, result.Success)
	assert.NotNil(t, result.Error)
	assert.Equal(t, "API error", result.Error.Message)
	assert.Equal(t, "ai_error", result.Error.Type)
}

func TestExecutor_ExecuteWithToolsAndUsage(t *testing.T) {
	// Create a client that returns a response with usage information.
	client := &mockClient{
		responses: []*types.Response{
			{
				Content:    "Final response with usage",
				StopReason: types.StopReasonEndTurn,
				Usage: &types.Usage{
					InputTokens:         100,
					OutputTokens:        50,
					TotalTokens:         150,
					CacheReadTokens:     20,
					CacheCreationTokens: 10,
				},
			},
		},
	}
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				DefaultProvider: "mock",
			},
		},
	}

	// Create registry with a tool.
	registry := tools.NewRegistry()
	mockToolInstance := &mockTool{
		name:        "test_tool",
		description: "A test tool",
	}
	err := registry.Register(mockToolInstance)
	require.NoError(t, err)

	toolExecutor := tools.NewExecutor(registry, nil, 30*time.Second)
	exec := NewExecutor(client, toolExecutor, atmosConfig)

	result := exec.Execute(context.Background(), Options{
		Prompt:       "Test prompt with tools",
		ToolsEnabled: true,
	})

	assert.True(t, result.Success)
	assert.Equal(t, "Final response with usage", result.Response)
	assert.Equal(t, int64(100), result.Tokens.Prompt)
	assert.Equal(t, int64(50), result.Tokens.Completion)
	assert.Equal(t, int64(150), result.Tokens.Total)
	assert.Equal(t, int64(20), result.Tokens.Cached)
	assert.Equal(t, int64(10), result.Tokens.CacheCreation)
	assert.Equal(t, types.StopReasonEndTurn, result.Metadata.StopReason)
}

func TestExecutor_ExecuteWithToolsMultipleRounds(t *testing.T) {
	// Create a client that returns tool use request first, then final response.
	client := &mockClient{
		responses: []*types.Response{
			{
				Content:    "I'll use a tool",
				StopReason: types.StopReasonToolUse,
				ToolCalls: []types.ToolCall{
					{
						ID:    "call-1",
						Name:  "test_tool",
						Input: map[string]interface{}{"param": "value"},
					},
				},
				Usage: &types.Usage{
					InputTokens:  50,
					OutputTokens: 25,
					TotalTokens:  75,
				},
			},
			{
				Content:    "Final response after tool use",
				StopReason: types.StopReasonEndTurn,
				Usage: &types.Usage{
					InputTokens:  100,
					OutputTokens: 50,
					TotalTokens:  150,
				},
			},
		},
	}
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				DefaultProvider: "mock",
			},
		},
	}

	// Create registry with a tool that returns successfully.
	registry := tools.NewRegistry()
	mockToolInstance := &mockTool{
		name:        "test_tool",
		description: "A test tool",
		executeFunc: func(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
			return &tools.Result{
				Success: true,
				Output:  "Tool executed successfully",
				Data:    map[string]interface{}{"result": "success"},
			}, nil
		},
	}
	err := registry.Register(mockToolInstance)
	require.NoError(t, err)

	toolExecutor := tools.NewExecutor(registry, nil, 30*time.Second)
	exec := NewExecutor(client, toolExecutor, atmosConfig)

	result := exec.Execute(context.Background(), Options{
		Prompt:       "Test prompt with tools",
		ToolsEnabled: true,
	})

	assert.True(t, result.Success)
	assert.Contains(t, result.Response, "Final response after tool use")
	assert.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "test_tool", result.ToolCalls[0].Tool)
	assert.True(t, result.ToolCalls[0].Success)

	// Usage should be combined.
	assert.Equal(t, int64(150), result.Tokens.Prompt)    // 50 + 100
	assert.Equal(t, int64(75), result.Tokens.Completion) // 25 + 50
	assert.Equal(t, int64(225), result.Tokens.Total)     // 75 + 150
}

func TestExecutor_ExecuteWithToolsToolError(t *testing.T) {
	// Create a client that returns tool use request.
	client := &mockClient{
		responses: []*types.Response{
			{
				Content:    "I'll use a tool",
				StopReason: types.StopReasonToolUse,
				ToolCalls: []types.ToolCall{
					{
						ID:    "call-1",
						Name:  "test_tool",
						Input: map[string]interface{}{"param": "value"},
					},
				},
			},
			{
				Content:    "Tool failed, here's my response",
				StopReason: types.StopReasonEndTurn,
			},
		},
	}
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				DefaultProvider: "mock",
			},
		},
	}

	// Create registry with a tool that returns an error.
	registry := tools.NewRegistry()
	mockToolInstance := &mockTool{
		name:        "test_tool",
		description: "A test tool that fails",
		executeFunc: func(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
			return nil, errors.New("tool execution failed")
		},
	}
	err := registry.Register(mockToolInstance)
	require.NoError(t, err)

	toolExecutor := tools.NewExecutor(registry, nil, 30*time.Second)
	exec := NewExecutor(client, toolExecutor, atmosConfig)

	result := exec.Execute(context.Background(), Options{
		Prompt:       "Test prompt with tools",
		ToolsEnabled: true,
	})

	assert.True(t, result.Success)
	assert.Contains(t, result.Response, "Tool failed")
	assert.Len(t, result.ToolCalls, 1)
	assert.False(t, result.ToolCalls[0].Success)
	assert.Contains(t, result.ToolCalls[0].Error, "tool execution failed")
}
