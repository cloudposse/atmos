package bedrock

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/agent/base"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// mockTool implements the tools.Tool interface for testing.
type mockTool struct {
	name               string
	description        string
	parameters         []tools.Parameter
	requiresPermission bool
	isRestricted       bool
}

func (m *mockTool) Name() string                  { return m.name }
func (m *mockTool) Description() string           { return m.description }
func (m *mockTool) Parameters() []tools.Parameter { return m.parameters }
func (m *mockTool) RequiresPermission() bool      { return m.requiresPermission }
func (m *mockTool) IsRestricted() bool            { return m.isRestricted }
func (m *mockTool) Execute(_ context.Context, _ map[string]interface{}) (*tools.Result, error) {
	return &tools.Result{Success: true}, nil
}

func TestExtractConfig(t *testing.T) {
	tests := []struct {
		name           string
		atmosConfig    *schema.AtmosConfiguration
		expectedConfig *base.Config
	}{
		{
			name: "Default configuration",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{},
				},
			},
			expectedConfig: &base.Config{
				Enabled:   false,
				Model:     "anthropic.claude-sonnet-4-20250514-v2:0",
				BaseURL:   "us-east-1", // Region stored in BaseURL.
				MaxTokens: 4096,
			},
		},
		{
			name: "Enabled configuration",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"bedrock": {
								Model:     "anthropic.claude-3-haiku-20240307-v1:0",
								MaxTokens: 8192,
								BaseURL:   "us-west-2",
							},
						},
					},
				},
			},
			expectedConfig: &base.Config{
				Enabled:   true,
				Model:     "anthropic.claude-3-haiku-20240307-v1:0",
				BaseURL:   "us-west-2",
				MaxTokens: 8192,
			},
		},
		{
			name: "Partial configuration",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"bedrock": {
								Model: "anthropic.claude-3-opus-20240229-v1:0",
							},
						},
					},
				},
			},
			expectedConfig: &base.Config{
				Enabled:   true,
				Model:     "anthropic.claude-3-opus-20240229-v1:0",
				BaseURL:   "us-east-1",
				MaxTokens: 4096,
			},
		},
		{
			name: "Custom region only",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"bedrock": {
								BaseURL: "eu-west-1",
							},
						},
					},
				},
			},
			expectedConfig: &base.Config{
				Enabled:   true,
				Model:     "anthropic.claude-sonnet-4-20250514-v2:0",
				BaseURL:   "eu-west-1",
				MaxTokens: 4096,
			},
		},
		{
			name: "Asia Pacific region",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"bedrock": {
								BaseURL:   "ap-southeast-1",
								Model:     "anthropic.claude-3-sonnet-20240229-v1:0",
								MaxTokens: 2048,
							},
						},
					},
				},
			},
			expectedConfig: &base.Config{
				Enabled:   true,
				Model:     "anthropic.claude-3-sonnet-20240229-v1:0",
				BaseURL:   "ap-southeast-1",
				MaxTokens: 2048,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := base.ExtractConfig(tt.atmosConfig, ProviderName, base.ProviderDefaults{
				Model:     DefaultModel,
				MaxTokens: DefaultMaxTokens,
				BaseURL:   DefaultRegion, // Region stored in BaseURL.
			})
			assert.Equal(t, tt.expectedConfig, config)
		})
	}
}

func TestNewClient_Disabled(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: false,
			},
		},
	}

	client, err := NewClient(context.TODO(), atmosConfig)
	assert.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "AI features are disabled")
}

func TestClientGetters(t *testing.T) {
	config := &base.Config{
		Enabled:   true,
		Model:     "anthropic.claude-sonnet-4-20250514-v2:0",
		BaseURL:   "us-east-1",
		MaxTokens: 4096,
	}

	client := &Client{
		client: nil, // We don't need a real client for testing getters.
		config: config,
		region: "us-east-1",
	}

	assert.Equal(t, "anthropic.claude-sonnet-4-20250514-v2:0", client.GetModel())
	assert.Equal(t, 4096, client.GetMaxTokens())
	assert.Equal(t, "us-east-1", client.GetRegion())
}

func TestDefaultConstants(t *testing.T) {
	assert.Equal(t, "bedrock", ProviderName)
	assert.Equal(t, 4096, DefaultMaxTokens)
	assert.Equal(t, "anthropic.claude-sonnet-4-20250514-v2:0", DefaultModel)
	assert.Equal(t, "us-east-1", DefaultRegion)
}

func TestConfig_AllFields(t *testing.T) {
	config := &base.Config{
		Enabled:   true,
		Model:     "test-model",
		BaseURL:   "ap-southeast-1",
		MaxTokens: 1000,
	}

	assert.True(t, config.Enabled)
	assert.Equal(t, "test-model", config.Model)
	assert.Equal(t, "ap-southeast-1", config.BaseURL)
	assert.Equal(t, 1000, config.MaxTokens)
}

func TestConvertMessagesToBedrockFormat_Empty(t *testing.T) {
	messages := []types.Message{}
	result := convertMessagesToBedrockFormat(messages)

	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestConvertMessagesToBedrockFormat_SingleUserMessage(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleUser, Content: "Hello, world!"},
	}

	result := convertMessagesToBedrockFormat(messages)

	require.Len(t, result, 1)
	assert.Equal(t, "user", result[0]["role"])
	assert.Equal(t, "Hello, world!", result[0]["content"])
}

func TestConvertMessagesToBedrockFormat_SingleAssistantMessage(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleAssistant, Content: "Hello! How can I help you?"},
	}

	result := convertMessagesToBedrockFormat(messages)

	require.Len(t, result, 1)
	assert.Equal(t, "assistant", result[0]["role"])
	assert.Equal(t, "Hello! How can I help you?", result[0]["content"])
}

func TestConvertMessagesToBedrockFormat_MultipleMessages(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleUser, Content: "What is 2+2?"},
		{Role: types.RoleAssistant, Content: "2+2 equals 4."},
		{Role: types.RoleUser, Content: "Thanks!"},
	}

	result := convertMessagesToBedrockFormat(messages)

	require.Len(t, result, 3)
	assert.Equal(t, "user", result[0]["role"])
	assert.Equal(t, "What is 2+2?", result[0]["content"])
	assert.Equal(t, "assistant", result[1]["role"])
	assert.Equal(t, "2+2 equals 4.", result[1]["content"])
	assert.Equal(t, "user", result[2]["role"])
	assert.Equal(t, "Thanks!", result[2]["content"])
}

func TestConvertMessagesToBedrockFormat_SystemMessageSkipped(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleSystem, Content: "You are a helpful assistant."},
		{Role: types.RoleUser, Content: "Hello"},
	}

	result := convertMessagesToBedrockFormat(messages)

	// System messages should be skipped (they go via system parameter, not messages array).
	require.Len(t, result, 1)
	assert.Equal(t, "user", result[0]["role"])
	assert.Equal(t, "Hello", result[0]["content"])
}

func TestConvertToolsToBedrockFormat_Empty(t *testing.T) {
	availableTools := []tools.Tool{}
	result := convertToolsToBedrockFormat(availableTools)

	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestConvertToolsToBedrockFormat_SingleTool(t *testing.T) {
	availableTools := []tools.Tool{
		&mockTool{
			name:        "test_tool",
			description: "A test tool",
			parameters: []tools.Parameter{
				{Name: "param1", Type: tools.ParamTypeString, Description: "First param", Required: true},
			},
		},
	}

	result := convertToolsToBedrockFormat(availableTools)

	require.Len(t, result, 1)
	assert.Equal(t, "test_tool", result[0]["name"])
	assert.Equal(t, "A test tool", result[0]["description"])

	inputSchema, ok := result[0]["input_schema"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "object", inputSchema["type"])

	properties, ok := inputSchema["properties"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, properties, "param1")

	required, ok := inputSchema["required"].([]string)
	require.True(t, ok)
	assert.Contains(t, required, "param1")
}

func TestConvertToolsToBedrockFormat_MultipleTools(t *testing.T) {
	availableTools := []tools.Tool{
		&mockTool{
			name:        "tool_a",
			description: "Tool A",
			parameters:  []tools.Parameter{},
		},
		&mockTool{
			name:        "tool_b",
			description: "Tool B",
			parameters: []tools.Parameter{
				{Name: "input", Type: tools.ParamTypeString, Description: "Input", Required: true},
			},
		},
	}

	result := convertToolsToBedrockFormat(availableTools)

	require.Len(t, result, 2)
	assert.Equal(t, "tool_a", result[0]["name"])
	assert.Equal(t, "tool_b", result[1]["name"])
}

func TestConvertToolsToBedrockFormat_AllParameterTypes(t *testing.T) {
	availableTools := []tools.Tool{
		&mockTool{
			name:        "comprehensive_tool",
			description: "Tool with all parameter types",
			parameters: []tools.Parameter{
				{Name: "string_param", Type: tools.ParamTypeString, Description: "String parameter", Required: true},
				{Name: "int_param", Type: tools.ParamTypeInt, Description: "Integer parameter", Required: true},
				{Name: "bool_param", Type: tools.ParamTypeBool, Description: "Boolean parameter", Required: false},
				{Name: "array_param", Type: tools.ParamTypeArray, Description: "Array parameter", Required: false},
				{Name: "object_param", Type: tools.ParamTypeObject, Description: "Object parameter", Required: false},
			},
		},
	}

	result := convertToolsToBedrockFormat(availableTools)

	require.Len(t, result, 1)
	assert.Equal(t, "comprehensive_tool", result[0]["name"])

	inputSchema, ok := result[0]["input_schema"].(map[string]interface{})
	require.True(t, ok)

	required, ok := inputSchema["required"].([]string)
	require.True(t, ok)
	assert.Len(t, required, 2)
	assert.Contains(t, required, "string_param")
	assert.Contains(t, required, "int_param")
}

func TestParseBedrockResponse_TextOnly(t *testing.T) {
	responseBody := `{
		"content": [
			{"type": "text", "text": "Hello! How can I help you?"}
		],
		"stop_reason": "end_turn",
		"usage": {
			"input_tokens": 10,
			"output_tokens": 8
		}
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	assert.Equal(t, "Hello! How can I help you?", result.Content)
	assert.Equal(t, types.StopReasonEndTurn, result.StopReason)
	assert.Empty(t, result.ToolCalls)
	require.NotNil(t, result.Usage)
	assert.Equal(t, int64(10), result.Usage.InputTokens)
	assert.Equal(t, int64(8), result.Usage.OutputTokens)
	assert.Equal(t, int64(18), result.Usage.TotalTokens)
}

func TestParseBedrockResponse_WithToolUse(t *testing.T) {
	responseBody := `{
		"content": [
			{"type": "text", "text": "I will search for that."},
			{
				"type": "tool_use",
				"id": "tool_call_123",
				"name": "search",
				"input": {"query": "test search", "limit": 5}
			}
		],
		"stop_reason": "tool_use",
		"usage": {
			"input_tokens": 20,
			"output_tokens": 15
		}
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	assert.Equal(t, "I will search for that.", result.Content)
	assert.Equal(t, types.StopReasonToolUse, result.StopReason)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "tool_call_123", result.ToolCalls[0].ID)
	assert.Equal(t, "search", result.ToolCalls[0].Name)
	assert.Equal(t, "test search", result.ToolCalls[0].Input["query"])
	assert.Equal(t, float64(5), result.ToolCalls[0].Input["limit"]) // JSON numbers are float64.
}

func TestParseBedrockResponse_MultipleToolUse(t *testing.T) {
	responseBody := `{
		"content": [
			{
				"type": "tool_use",
				"id": "call_1",
				"name": "read_file",
				"input": {"file": "test.txt"}
			},
			{
				"type": "tool_use",
				"id": "call_2",
				"name": "search",
				"input": {"query": "search term"}
			}
		],
		"stop_reason": "tool_use"
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	assert.Equal(t, types.StopReasonToolUse, result.StopReason)
	require.Len(t, result.ToolCalls, 2)
	assert.Equal(t, "read_file", result.ToolCalls[0].Name)
	assert.Equal(t, "search", result.ToolCalls[1].Name)
}

func TestParseBedrockResponse_StopReasonMaxTokens(t *testing.T) {
	responseBody := `{
		"content": [
			{"type": "text", "text": "Truncated response..."}
		],
		"stop_reason": "max_tokens"
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	assert.Equal(t, types.StopReasonMaxTokens, result.StopReason)
}

func TestParseBedrockResponse_UnknownStopReason(t *testing.T) {
	responseBody := `{
		"content": [
			{"type": "text", "text": "Response"}
		],
		"stop_reason": "unknown_reason"
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	// Unknown stop reason defaults to EndTurn.
	assert.Equal(t, types.StopReasonEndTurn, result.StopReason)
}

func TestParseBedrockResponse_InvalidJSON(t *testing.T) {
	responseBody := `{invalid json}`

	result, err := parseBedrockResponse([]byte(responseBody))

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to unmarshal")
}

func TestParseBedrockResponse_EmptyContent(t *testing.T) {
	responseBody := `{
		"content": [],
		"stop_reason": "end_turn"
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	assert.Empty(t, result.Content)
	assert.Empty(t, result.ToolCalls)
}

func TestParseBedrockResponse_MultipleTextBlocks(t *testing.T) {
	responseBody := `{
		"content": [
			{"type": "text", "text": "First part. "},
			{"type": "text", "text": "Second part. "},
			{"type": "text", "text": "Third part."}
		],
		"stop_reason": "end_turn"
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	assert.Equal(t, "First part. Second part. Third part.", result.Content)
}

func TestParseBedrockResponse_NoUsage(t *testing.T) {
	responseBody := `{
		"content": [
			{"type": "text", "text": "Response"}
		],
		"stop_reason": "end_turn",
		"usage": {
			"input_tokens": 0,
			"output_tokens": 0
		}
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	// When both tokens are 0, Usage should still be nil.
	assert.Nil(t, result.Usage)
}

func TestParseBedrockResponse_ToolUseWithEmptyInput(t *testing.T) {
	responseBody := `{
		"content": [
			{
				"type": "tool_use",
				"id": "call_empty",
				"name": "no_params_tool",
				"input": {}
			}
		],
		"stop_reason": "tool_use"
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	require.Len(t, result.ToolCalls, 1)
	assert.Empty(t, result.ToolCalls[0].Input)
}

func TestParseBedrockResponse_ComplexToolInput(t *testing.T) {
	responseBody := `{
		"content": [
			{
				"type": "tool_use",
				"id": "call_complex",
				"name": "describe_component",
				"input": {
					"component": "vpc",
					"stack": "prod-use1",
					"options": {
						"verbose": true,
						"limit": 10
					}
				}
			}
		],
		"stop_reason": "tool_use"
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "vpc", result.ToolCalls[0].Input["component"])
	assert.Equal(t, "prod-use1", result.ToolCalls[0].Input["stack"])

	options, ok := result.ToolCalls[0].Input["options"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, options["verbose"])
	assert.Equal(t, float64(10), options["limit"])
}

func TestClientGetters_AllRegions(t *testing.T) {
	regions := []string{
		"us-east-1",
		"us-west-2",
		"eu-west-1",
		"ap-southeast-1",
		"ap-northeast-1",
	}

	for _, region := range regions {
		t.Run(region, func(t *testing.T) {
			client := &Client{
				client: nil,
				config: &base.Config{
					Enabled:   true,
					Model:     DefaultModel,
					BaseURL:   region,
					MaxTokens: DefaultMaxTokens,
				},
				region: region,
			}

			assert.Equal(t, region, client.GetRegion())
		})
	}
}

func TestBedrockModels(t *testing.T) {
	// Test various Bedrock model IDs.
	models := []struct {
		modelID     string
		description string
	}{
		{"anthropic.claude-sonnet-4-20250514-v2:0", "Claude Sonnet 4"},
		{"anthropic.claude-3-haiku-20240307-v1:0", "Claude 3 Haiku"},
		{"anthropic.claude-3-opus-20240229-v1:0", "Claude 3 Opus"},
		{"anthropic.claude-3-sonnet-20240229-v1:0", "Claude 3 Sonnet"},
		{"anthropic.claude-v2:1", "Claude v2.1"},
	}

	for _, m := range models {
		t.Run(m.description, func(t *testing.T) {
			config := &base.Config{
				Enabled:   true,
				Model:     m.modelID,
				BaseURL:   DefaultRegion,
				MaxTokens: DefaultMaxTokens,
			}

			client := &Client{
				client: nil,
				config: config,
				region: DefaultRegion,
			}

			assert.Equal(t, m.modelID, client.GetModel())
		})
	}
}

func TestRequestBodyStructure(t *testing.T) {
	// Test that request body is structured correctly for Bedrock/Anthropic format.
	config := &base.Config{
		Enabled:   true,
		Model:     DefaultModel,
		BaseURL:   DefaultRegion,
		MaxTokens: 4096,
	}

	client := &Client{
		client: nil,
		config: config,
		region: DefaultRegion,
	}

	message := "Test message"
	requestBody := map[string]interface{}{
		"anthropic_version": "bedrock-2023-05-31",
		"max_tokens":        client.config.MaxTokens,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": message,
			},
		},
	}

	// Verify structure.
	assert.Equal(t, "bedrock-2023-05-31", requestBody["anthropic_version"])
	assert.Equal(t, 4096, requestBody["max_tokens"])

	messages, ok := requestBody["messages"].([]map[string]string)
	require.True(t, ok)
	require.Len(t, messages, 1)
	assert.Equal(t, "user", messages[0]["role"])
	assert.Equal(t, "Test message", messages[0]["content"])

	// Verify it can be marshaled to JSON.
	bodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)
	assert.NotEmpty(t, bodyBytes)
}

func TestRequestBodyWithTools(t *testing.T) {
	// Test that request body includes tools correctly.
	availableTools := []tools.Tool{
		&mockTool{
			name:        "test_tool",
			description: "A test tool",
			parameters: []tools.Parameter{
				{Name: "param1", Type: tools.ParamTypeString, Description: "First param", Required: true},
			},
		},
	}

	bedrockTools := convertToolsToBedrockFormat(availableTools)

	requestBody := map[string]interface{}{
		"anthropic_version": "bedrock-2023-05-31",
		"max_tokens":        4096,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": "Test message",
			},
		},
		"tools": bedrockTools,
	}

	// Verify tools are included.
	tools, ok := requestBody["tools"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, tools, 1)
	assert.Equal(t, "test_tool", tools[0]["name"])

	// Verify it can be marshaled to JSON.
	bodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)
	assert.NotEmpty(t, bodyBytes)
	assert.Contains(t, string(bodyBytes), "test_tool")
}

func TestAWSRegionValidation(t *testing.T) {
	// Test that various AWS regions are accepted.
	validRegions := []string{
		"us-east-1",
		"us-east-2",
		"us-west-1",
		"us-west-2",
		"eu-west-1",
		"eu-west-2",
		"eu-west-3",
		"eu-central-1",
		"eu-north-1",
		"ap-southeast-1",
		"ap-southeast-2",
		"ap-northeast-1",
		"ap-northeast-2",
		"ap-south-1",
		"sa-east-1",
		"ca-central-1",
	}

	for _, region := range validRegions {
		t.Run(region, func(t *testing.T) {
			config := base.ExtractConfig(&schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"bedrock": {
								BaseURL: region,
							},
						},
					},
				},
			}, ProviderName, base.ProviderDefaults{
				Model:     DefaultModel,
				MaxTokens: DefaultMaxTokens,
				BaseURL:   DefaultRegion,
			})

			assert.Equal(t, region, config.BaseURL)
		})
	}
}
