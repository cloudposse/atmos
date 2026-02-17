package openaicompat

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/openai/openai-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
)

// mockTool implements the tools.Tool interface for testing.
type mockTool struct {
	name               string
	description        string
	params             []tools.Parameter
	requiresPermission bool
	isRestricted       bool
}

func (m *mockTool) Name() string                  { return m.name }
func (m *mockTool) Description() string           { return m.description }
func (m *mockTool) Parameters() []tools.Parameter { return m.params }
func (m *mockTool) RequiresPermission() bool      { return m.requiresPermission }
func (m *mockTool) IsRestricted() bool            { return m.isRestricted }
func (m *mockTool) Execute(_ context.Context, _ map[string]interface{}) (*tools.Result, error) {
	return &tools.Result{Success: true}, nil
}

func TestConvertMessagesToOpenAIFormat_Empty(t *testing.T) {
	messages := []types.Message{}

	result := ConvertMessagesToOpenAIFormat(messages)

	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestConvertMessagesToOpenAIFormat_SingleUserMessage(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleUser, Content: "Hello, world!"},
	}

	result := ConvertMessagesToOpenAIFormat(messages)

	assert.Len(t, result, 1)
}

func TestConvertMessagesToOpenAIFormat_SingleAssistantMessage(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleAssistant, Content: "Hello! How can I help you?"},
	}

	result := ConvertMessagesToOpenAIFormat(messages)

	assert.Len(t, result, 1)
}

func TestConvertMessagesToOpenAIFormat_SingleSystemMessage(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleSystem, Content: "You are a helpful assistant."},
	}

	result := ConvertMessagesToOpenAIFormat(messages)

	assert.Len(t, result, 1)
}

func TestConvertMessagesToOpenAIFormat_MultipleMessages(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleSystem, Content: "You are a helpful assistant."},
		{Role: types.RoleUser, Content: "What is 2+2?"},
		{Role: types.RoleAssistant, Content: "2+2 equals 4."},
		{Role: types.RoleUser, Content: "Thanks!"},
	}

	result := ConvertMessagesToOpenAIFormat(messages)

	assert.Len(t, result, 4)
}

func TestConvertMessagesToOpenAIFormat_UnknownRole(t *testing.T) {
	messages := []types.Message{
		{Role: "unknown", Content: "This should be skipped"},
	}

	result := ConvertMessagesToOpenAIFormat(messages)

	// Unknown roles are skipped.
	assert.Empty(t, result)
}

func TestConvertToolsToOpenAIFormat_Empty(t *testing.T) {
	availableTools := []tools.Tool{}

	result := ConvertToolsToOpenAIFormat(availableTools)

	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestConvertToolsToOpenAIFormat_SingleTool(t *testing.T) {
	availableTools := []tools.Tool{
		&mockTool{
			name:        "test_tool",
			description: "A test tool",
			params: []tools.Parameter{
				{Name: "param1", Type: tools.ParamTypeString, Description: "First param", Required: true},
			},
		},
	}

	result := ConvertToolsToOpenAIFormat(availableTools)

	assert.Len(t, result, 1)
	assert.Equal(t, "test_tool", result[0].Function.Name)
	assert.Equal(t, "A test tool", result[0].Function.Description.Value)
}

func TestConvertToolsToOpenAIFormat_MultipleTools(t *testing.T) {
	availableTools := []tools.Tool{
		&mockTool{
			name:        "tool_a",
			description: "Tool A",
			params:      []tools.Parameter{},
		},
		&mockTool{
			name:        "tool_b",
			description: "Tool B",
			params: []tools.Parameter{
				{Name: "input", Type: tools.ParamTypeString, Description: "Input", Required: true},
			},
		},
	}

	result := ConvertToolsToOpenAIFormat(availableTools)

	assert.Len(t, result, 2)
	assert.Equal(t, "tool_a", result[0].Function.Name)
	assert.Equal(t, "tool_b", result[1].Function.Name)
}

func TestParseOpenAIResponse_EmptyChoices(t *testing.T) {
	response := &openai.ChatCompletion{
		Choices: []openai.ChatCompletionChoice{},
	}

	result, err := ParseOpenAIResponse(response)

	assert.Nil(t, result)
	assert.Error(t, err)
}

func TestParseOpenAIResponse_StopFinishReason(t *testing.T) {
	response := &openai.ChatCompletion{
		Choices: []openai.ChatCompletionChoice{
			{
				FinishReason: "stop",
				Message: openai.ChatCompletionMessage{
					Content: "Hello!",
				},
			},
		},
	}

	result, err := ParseOpenAIResponse(response)

	require.NoError(t, err)
	assert.Equal(t, "Hello!", result.Content)
	assert.Equal(t, types.StopReasonEndTurn, result.StopReason)
}

func TestParseOpenAIResponse_ToolCallsFinishReason(t *testing.T) {
	response := &openai.ChatCompletion{
		Choices: []openai.ChatCompletionChoice{
			{
				FinishReason: "tool_calls",
				Message: openai.ChatCompletionMessage{
					Content: "",
					ToolCalls: []openai.ChatCompletionMessageToolCall{
						{
							ID: "call_123",
							Function: openai.ChatCompletionMessageToolCallFunction{
								Name:      "test_tool",
								Arguments: `{"param1": "value1"}`,
							},
						},
					},
				},
			},
		},
	}

	result, err := ParseOpenAIResponse(response)

	require.NoError(t, err)
	assert.Equal(t, types.StopReasonToolUse, result.StopReason)
	assert.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "call_123", result.ToolCalls[0].ID)
	assert.Equal(t, "test_tool", result.ToolCalls[0].Name)
	assert.Equal(t, "value1", result.ToolCalls[0].Input["param1"])
}

func TestParseOpenAIResponse_LengthFinishReason(t *testing.T) {
	response := &openai.ChatCompletion{
		Choices: []openai.ChatCompletionChoice{
			{
				FinishReason: "length",
				Message: openai.ChatCompletionMessage{
					Content: "Truncated...",
				},
			},
		},
	}

	result, err := ParseOpenAIResponse(response)

	require.NoError(t, err)
	assert.Equal(t, types.StopReasonMaxTokens, result.StopReason)
}

func TestParseOpenAIResponse_UnknownFinishReason(t *testing.T) {
	response := &openai.ChatCompletion{
		Choices: []openai.ChatCompletionChoice{
			{
				FinishReason: "unknown_reason",
				Message: openai.ChatCompletionMessage{
					Content: "Content",
				},
			},
		},
	}

	result, err := ParseOpenAIResponse(response)

	require.NoError(t, err)
	// Unknown finish reason defaults to EndTurn.
	assert.Equal(t, types.StopReasonEndTurn, result.StopReason)
}

func TestParseOpenAIResponse_WithUsage(t *testing.T) {
	response := &openai.ChatCompletion{
		Choices: []openai.ChatCompletionChoice{
			{
				FinishReason: "stop",
				Message: openai.ChatCompletionMessage{
					Content: "Response",
				},
			},
		},
		Usage: openai.CompletionUsage{
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
		},
	}

	result, err := ParseOpenAIResponse(response)

	require.NoError(t, err)
	require.NotNil(t, result.Usage)
	assert.Equal(t, int64(100), result.Usage.InputTokens)
	assert.Equal(t, int64(50), result.Usage.OutputTokens)
	assert.Equal(t, int64(150), result.Usage.TotalTokens)
}

func TestParseOpenAIResponse_InvalidToolArguments(t *testing.T) {
	response := &openai.ChatCompletion{
		Choices: []openai.ChatCompletionChoice{
			{
				FinishReason: "tool_calls",
				Message: openai.ChatCompletionMessage{
					ToolCalls: []openai.ChatCompletionMessageToolCall{
						{
							ID: "call_123",
							Function: openai.ChatCompletionMessageToolCallFunction{
								Name:      "test_tool",
								Arguments: `{invalid json}`,
							},
						},
					},
				},
			},
		},
	}

	result, err := ParseOpenAIResponse(response)

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse tool arguments")
}

func TestParseOpenAIResponse_EmptyToolArguments(t *testing.T) {
	response := &openai.ChatCompletion{
		Choices: []openai.ChatCompletionChoice{
			{
				FinishReason: "tool_calls",
				Message: openai.ChatCompletionMessage{
					ToolCalls: []openai.ChatCompletionMessageToolCall{
						{
							ID: "call_123",
							Function: openai.ChatCompletionMessageToolCallFunction{
								Name:      "test_tool",
								Arguments: "",
							},
						},
					},
				},
			},
		},
	}

	result, err := ParseOpenAIResponse(response)

	require.NoError(t, err)
	assert.Len(t, result.ToolCalls, 1)
	assert.Nil(t, result.ToolCalls[0].Input)
}

func TestParseOpenAIResponse_MultipleToolCalls(t *testing.T) {
	response := &openai.ChatCompletion{
		Choices: []openai.ChatCompletionChoice{
			{
				FinishReason: "tool_calls",
				Message: openai.ChatCompletionMessage{
					ToolCalls: []openai.ChatCompletionMessageToolCall{
						{
							ID: "call_1",
							Function: openai.ChatCompletionMessageToolCallFunction{
								Name:      "tool_a",
								Arguments: `{"x": 1}`,
							},
						},
						{
							ID: "call_2",
							Function: openai.ChatCompletionMessageToolCallFunction{
								Name:      "tool_b",
								Arguments: `{"y": 2}`,
							},
						},
					},
				},
			},
		},
	}

	result, err := ParseOpenAIResponse(response)

	require.NoError(t, err)
	assert.Len(t, result.ToolCalls, 2)
	assert.Equal(t, "tool_a", result.ToolCalls[0].Name)
	assert.Equal(t, "tool_b", result.ToolCalls[1].Name)
}

func TestRequiresMaxCompletionTokens(t *testing.T) {
	tests := []struct {
		model    string
		expected bool
	}{
		// Models that require max_completion_tokens.
		{"gpt-5", true},
		{"gpt-5-turbo", true},
		{"gpt-5-1106-preview", true},
		{"o1-preview", true},
		{"o1-mini", true},
		{"chatgpt-4o-latest", true},

		// Models that use max_tokens.
		{"gpt-4", false},
		{"gpt-4o", false},
		{"gpt-4-turbo", false},
		{"gpt-3.5-turbo", false},
		{"gpt-4o-mini", false},
		{"o1", false}, // Only o1-preview and o1-mini.
		{"llama3", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			result := RequiresMaxCompletionTokens(tt.model)
			assert.Equal(t, tt.expected, result, "model: %s", tt.model)
		})
	}
}

func TestSetTokenLimit_MaxCompletionTokens(t *testing.T) {
	params := &openai.ChatCompletionNewParams{}

	SetTokenLimit(params, "gpt-5", 4096)

	// Should set MaxCompletionTokens, not MaxTokens.
	assert.Equal(t, int64(4096), params.MaxCompletionTokens.Value)
	assert.False(t, params.MaxTokens.Valid())
}

func TestSetTokenLimit_MaxTokens(t *testing.T) {
	params := &openai.ChatCompletionNewParams{}

	SetTokenLimit(params, "gpt-4o", 4096)

	// Should set MaxTokens, not MaxCompletionTokens.
	assert.Equal(t, int64(4096), params.MaxTokens.Value)
	assert.False(t, params.MaxCompletionTokens.Valid())
}

func TestSetTokenLimit_AllModels(t *testing.T) {
	tests := []struct {
		model                     string
		expectMaxCompletionTokens bool
	}{
		{"gpt-5", true},
		{"o1-preview", true},
		{"o1-mini", true},
		{"chatgpt-4o-latest", true},
		{"gpt-4o", false},
		{"gpt-4-turbo", false},
		{"gpt-3.5-turbo", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			params := &openai.ChatCompletionNewParams{}
			SetTokenLimit(params, tt.model, 8192)

			if tt.expectMaxCompletionTokens {
				assert.True(t, params.MaxCompletionTokens.Valid(), "model %s should use MaxCompletionTokens", tt.model)
				assert.Equal(t, int64(8192), params.MaxCompletionTokens.Value)
				assert.False(t, params.MaxTokens.Valid())
			} else {
				assert.True(t, params.MaxTokens.Valid(), "model %s should use MaxTokens", tt.model)
				assert.Equal(t, int64(8192), params.MaxTokens.Value)
				assert.False(t, params.MaxCompletionTokens.Valid())
			}
		})
	}
}

func TestConvertToolsToOpenAIFormat_ComplexParameters(t *testing.T) {
	availableTools := []tools.Tool{
		&mockTool{
			name:        "describe_component",
			description: "Describe an Atmos component",
			params: []tools.Parameter{
				{Name: "component", Type: tools.ParamTypeString, Description: "Component name", Required: true},
				{Name: "stack", Type: tools.ParamTypeString, Description: "Stack name", Required: true},
				{Name: "verbose", Type: tools.ParamTypeBool, Description: "Verbose output", Required: false, Default: false},
				{Name: "limit", Type: tools.ParamTypeInt, Description: "Limit results", Required: false, Default: 10},
			},
		},
	}

	result := ConvertToolsToOpenAIFormat(availableTools)

	require.Len(t, result, 1)
	assert.Equal(t, "describe_component", result[0].Function.Name)

	// Verify the parameters are converted correctly.
	params := result[0].Function.Parameters
	require.NotNil(t, params)

	// Check properties exist.
	props, ok := params["properties"]
	assert.True(t, ok)
	assert.NotNil(t, props)

	// Check required fields.
	required, ok := params["required"]
	assert.True(t, ok)
	requiredList, ok := required.([]string)
	assert.True(t, ok)
	assert.Contains(t, requiredList, "component")
	assert.Contains(t, requiredList, "stack")
}

// TestParseOpenAIResponse_ComplexToolArguments tests parsing of complex nested arguments.
func TestParseOpenAIResponse_ComplexToolArguments(t *testing.T) {
	args := map[string]interface{}{
		"component": "vpc",
		"stack":     "prod-use1",
		"options": map[string]interface{}{
			"verbose": true,
			"limit":   10,
		},
	}
	argsJSON, _ := json.Marshal(args)

	response := &openai.ChatCompletion{
		Choices: []openai.ChatCompletionChoice{
			{
				FinishReason: "tool_calls",
				Message: openai.ChatCompletionMessage{
					ToolCalls: []openai.ChatCompletionMessageToolCall{
						{
							ID: "call_complex",
							Function: openai.ChatCompletionMessageToolCallFunction{
								Name:      "describe_component",
								Arguments: string(argsJSON),
							},
						},
					},
				},
			},
		},
	}

	result, err := ParseOpenAIResponse(response)

	require.NoError(t, err)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "vpc", result.ToolCalls[0].Input["component"])
	assert.Equal(t, "prod-use1", result.ToolCalls[0].Input["stack"])

	options, ok := result.ToolCalls[0].Input["options"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, options["verbose"])
	assert.Equal(t, float64(10), options["limit"]) // JSON numbers are float64.
}
