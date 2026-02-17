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
				Model:     "anthropic.claude-sonnet-4-5-20250929-v1:0",
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
				Model:     "anthropic.claude-sonnet-4-5-20250929-v1:0",
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
		Model:     "anthropic.claude-sonnet-4-5-20250929-v1:0",
		BaseURL:   "us-east-1",
		MaxTokens: 4096,
	}

	client := &Client{
		client: nil, // We don't need a real client for testing getters.
		config: config,
		region: "us-east-1",
	}

	assert.Equal(t, "anthropic.claude-sonnet-4-5-20250929-v1:0", client.GetModel())
	assert.Equal(t, 4096, client.GetMaxTokens())
	assert.Equal(t, "us-east-1", client.GetRegion())
}

func TestDefaultConstants(t *testing.T) {
	assert.Equal(t, "bedrock", ProviderName)
	assert.Equal(t, 4096, DefaultMaxTokens)
	assert.Equal(t, "anthropic.claude-sonnet-4-5-20250929-v1:0", DefaultModel)
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
		{"anthropic.claude-sonnet-4-5-20250929-v1:0", "Claude Sonnet 4.5"},
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

func TestExtractConfig_NilProviders(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled:   true,
				Providers: nil,
			},
		},
	}

	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:     DefaultModel,
		MaxTokens: DefaultMaxTokens,
		BaseURL:   DefaultRegion,
	})

	// Should use defaults when providers is nil.
	assert.True(t, config.Enabled)
	assert.Equal(t, DefaultModel, config.Model)
	assert.Equal(t, DefaultMaxTokens, config.MaxTokens)
	assert.Equal(t, DefaultRegion, config.BaseURL)
}

func TestExtractConfig_DifferentProviderOnly(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"openai": {
						Model: "gpt-4o",
					},
				},
			},
		},
	}

	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:     DefaultModel,
		MaxTokens: DefaultMaxTokens,
		BaseURL:   DefaultRegion,
	})

	// Should use defaults when this provider is not configured.
	assert.True(t, config.Enabled)
	assert.Equal(t, DefaultModel, config.Model)
	assert.Equal(t, DefaultMaxTokens, config.MaxTokens)
	assert.Equal(t, DefaultRegion, config.BaseURL)
}

func TestExtractConfig_NilProviderConfig(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"bedrock": nil, // Explicitly nil provider config.
				},
			},
		},
	}

	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:     DefaultModel,
		MaxTokens: DefaultMaxTokens,
		BaseURL:   DefaultRegion,
	})

	// Should use defaults when provider config is nil.
	assert.True(t, config.Enabled)
	assert.Equal(t, DefaultModel, config.Model)
	assert.Equal(t, DefaultMaxTokens, config.MaxTokens)
	assert.Equal(t, DefaultRegion, config.BaseURL)
}

func TestConvertMessagesToBedrockFormat_UnknownRole(t *testing.T) {
	messages := []types.Message{
		{Role: "unknown", Content: "This should be skipped"},
		{Role: types.RoleUser, Content: "Valid message"},
	}

	result := convertMessagesToBedrockFormat(messages)

	// Unknown roles should be skipped.
	require.Len(t, result, 1)
	assert.Equal(t, "user", result[0]["role"])
	assert.Equal(t, "Valid message", result[0]["content"])
}

func TestConvertMessagesToBedrockFormat_EmptyContent(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleUser, Content: ""},
		{Role: types.RoleAssistant, Content: ""},
	}

	result := convertMessagesToBedrockFormat(messages)

	// Empty content messages should still be converted.
	require.Len(t, result, 2)
	assert.Equal(t, "", result[0]["content"])
	assert.Equal(t, "", result[1]["content"])
}

func TestConvertToolsToBedrockFormat_NoParameters(t *testing.T) {
	availableTools := []tools.Tool{
		&mockTool{
			name:        "no_params_tool",
			description: "Tool without parameters",
			parameters:  []tools.Parameter{},
		},
	}

	result := convertToolsToBedrockFormat(availableTools)

	require.Len(t, result, 1)
	assert.Equal(t, "no_params_tool", result[0]["name"])

	inputSchema, ok := result[0]["input_schema"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "object", inputSchema["type"])

	properties, ok := inputSchema["properties"].(map[string]interface{})
	require.True(t, ok)
	assert.Empty(t, properties)

	required, ok := inputSchema["required"].([]string)
	require.True(t, ok)
	assert.Empty(t, required)
}

func TestConvertToolsToBedrockFormat_OnlyOptionalParameters(t *testing.T) {
	availableTools := []tools.Tool{
		&mockTool{
			name:        "optional_params_tool",
			description: "Tool with only optional parameters",
			parameters: []tools.Parameter{
				{Name: "opt1", Type: tools.ParamTypeString, Description: "Optional 1", Required: false},
				{Name: "opt2", Type: tools.ParamTypeInt, Description: "Optional 2", Required: false},
			},
		},
	}

	result := convertToolsToBedrockFormat(availableTools)

	require.Len(t, result, 1)
	inputSchema, ok := result[0]["input_schema"].(map[string]interface{})
	require.True(t, ok)

	required, ok := inputSchema["required"].([]string)
	require.True(t, ok)
	assert.Empty(t, required) // No required parameters.

	properties, ok := inputSchema["properties"].(map[string]interface{})
	require.True(t, ok)
	assert.Len(t, properties, 2) // Both optional parameters should be in properties.
}

func TestParseBedrockResponse_MixedContentTypes(t *testing.T) {
	// Test response with unknown content types mixed in.
	responseBody := `{
		"content": [
			{"type": "text", "text": "Hello"},
			{"type": "image", "data": "base64data"},
			{"type": "text", "text": " World"},
			{"type": "unknown_type", "data": "ignored"}
		],
		"stop_reason": "end_turn"
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	// Only text content should be extracted.
	assert.Equal(t, "Hello World", result.Content)
}

func TestParseBedrockResponse_ToolUseWithNullInput(t *testing.T) {
	responseBody := `{
		"content": [
			{
				"type": "tool_use",
				"id": "call_null",
				"name": "tool_with_null_input",
				"input": null
			}
		],
		"stop_reason": "tool_use"
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	require.Len(t, result.ToolCalls, 1)
	assert.Nil(t, result.ToolCalls[0].Input)
}

func TestParseBedrockResponse_LargeUsage(t *testing.T) {
	responseBody := `{
		"content": [
			{"type": "text", "text": "Response"}
		],
		"stop_reason": "end_turn",
		"usage": {
			"input_tokens": 1000000,
			"output_tokens": 500000
		}
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	require.NotNil(t, result.Usage)
	assert.Equal(t, int64(1000000), result.Usage.InputTokens)
	assert.Equal(t, int64(500000), result.Usage.OutputTokens)
	assert.Equal(t, int64(1500000), result.Usage.TotalTokens)
}

func TestClientGetters_CustomMaxTokens(t *testing.T) {
	tests := []struct {
		name      string
		maxTokens int
	}{
		{"Small", 1024},
		{"Default", 4096},
		{"Large", 8192},
		{"Very Large", 200000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				client: nil,
				config: &base.Config{
					Enabled:   true,
					Model:     DefaultModel,
					BaseURL:   DefaultRegion,
					MaxTokens: tt.maxTokens,
				},
				region: DefaultRegion,
			}

			assert.Equal(t, tt.maxTokens, client.GetMaxTokens())
		})
	}
}

func TestExtractConfig_AllOverrides(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"bedrock": {
						Model:     "anthropic.claude-3-opus-20240229-v1:0",
						MaxTokens: 32768,
						BaseURL:   "eu-central-1",
					},
				},
			},
		},
	}

	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:     DefaultModel,
		MaxTokens: DefaultMaxTokens,
		BaseURL:   DefaultRegion,
	})

	assert.True(t, config.Enabled)
	assert.Equal(t, "anthropic.claude-3-opus-20240229-v1:0", config.Model)
	assert.Equal(t, 32768, config.MaxTokens)
	assert.Equal(t, "eu-central-1", config.BaseURL)
}

func TestNewClient_EmptyRegion(t *testing.T) {
	// Test that empty region falls back to default.
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"bedrock": {
						BaseURL: "", // Empty region.
					},
				},
			},
		},
	}

	// Note: This will actually try to load AWS config, which may fail in test environment.
	// We're testing the fallback logic, not the AWS SDK.
	client, _ := NewClient(context.Background(), atmosConfig)
	// If client creation succeeds, verify default region is used.
	if client != nil {
		assert.Equal(t, DefaultRegion, client.GetRegion())
	}
}

func TestNewClient_NilConfig(t *testing.T) {
	// Test nil configuration.
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: false,
			},
		},
	}

	client, err := NewClient(context.Background(), atmosConfig)
	assert.Error(t, err)
	assert.Nil(t, client)
}

func TestConvertMessagesToBedrockFormat_OnlySystemMessages(t *testing.T) {
	// Test all system messages (should all be skipped).
	messages := []types.Message{
		{Role: types.RoleSystem, Content: "System message 1"},
		{Role: types.RoleSystem, Content: "System message 2"},
	}

	result := convertMessagesToBedrockFormat(messages)

	// All system messages should be skipped.
	assert.Empty(t, result)
}

func TestConvertMessagesToBedrockFormat_MixedRoles(t *testing.T) {
	// Test mixed roles including system (which should be skipped).
	messages := []types.Message{
		{Role: types.RoleSystem, Content: "System"},
		{Role: types.RoleUser, Content: "User 1"},
		{Role: types.RoleSystem, Content: "Another system"},
		{Role: types.RoleAssistant, Content: "Assistant 1"},
		{Role: types.RoleUser, Content: "User 2"},
	}

	result := convertMessagesToBedrockFormat(messages)

	// Only user and assistant messages should be included.
	require.Len(t, result, 3)
	assert.Equal(t, "user", result[0]["role"])
	assert.Equal(t, "User 1", result[0]["content"])
	assert.Equal(t, "assistant", result[1]["role"])
	assert.Equal(t, "Assistant 1", result[1]["content"])
	assert.Equal(t, "user", result[2]["role"])
	assert.Equal(t, "User 2", result[2]["content"])
}

func TestConvertMessagesToBedrockFormat_LongContent(t *testing.T) {
	// Test with very long content.
	longContent := string(make([]byte, 10000))
	messages := []types.Message{
		{Role: types.RoleUser, Content: longContent},
	}

	result := convertMessagesToBedrockFormat(messages)

	require.Len(t, result, 1)
	assert.Equal(t, longContent, result[0]["content"])
}

func TestConvertMessagesToBedrockFormat_SpecialCharacters(t *testing.T) {
	// Test with special characters.
	messages := []types.Message{
		{Role: types.RoleUser, Content: "Hello\nWorld\t!@#$%^&*()"},
		{Role: types.RoleAssistant, Content: `{"json": "content"}`},
	}

	result := convertMessagesToBedrockFormat(messages)

	require.Len(t, result, 2)
	assert.Equal(t, "Hello\nWorld\t!@#$%^&*()", result[0]["content"])
	assert.Equal(t, `{"json": "content"}`, result[1]["content"])
}

func TestConvertToolsToBedrockFormat_ToolWithDescription(t *testing.T) {
	// Test tool with long description.
	availableTools := []tools.Tool{
		&mockTool{
			name:        "complex_tool",
			description: "This is a very long description that explains in detail what the tool does and why it's useful for various scenarios.",
			parameters: []tools.Parameter{
				{Name: "param", Type: tools.ParamTypeString, Description: "A parameter", Required: true},
			},
		},
	}

	result := convertToolsToBedrockFormat(availableTools)

	require.Len(t, result, 1)
	assert.Contains(t, result[0]["description"], "very long description")
}

func TestConvertToolsToBedrockFormat_ToolRequiresPermission(t *testing.T) {
	// Test that tool permission flags don't affect conversion.
	availableTools := []tools.Tool{
		&mockTool{
			name:               "restricted_tool",
			description:        "A restricted tool",
			parameters:         []tools.Parameter{},
			requiresPermission: true,
			isRestricted:       true,
		},
	}

	result := convertToolsToBedrockFormat(availableTools)

	require.Len(t, result, 1)
	assert.Equal(t, "restricted_tool", result[0]["name"])
	// Permission flags shouldn't affect the Bedrock format.
}

func TestConvertToolsToBedrockFormat_NestedParameters(t *testing.T) {
	// Test tool with object parameter type.
	availableTools := []tools.Tool{
		&mockTool{
			name:        "nested_tool",
			description: "Tool with nested parameters",
			parameters: []tools.Parameter{
				{
					Name:        "config",
					Type:        tools.ParamTypeObject,
					Description: "Configuration object",
					Required:    true,
				},
			},
		},
	}

	result := convertToolsToBedrockFormat(availableTools)

	require.Len(t, result, 1)
	inputSchema, ok := result[0]["input_schema"].(map[string]interface{})
	require.True(t, ok)

	properties, ok := inputSchema["properties"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, properties, "config")
}

func TestParseBedrockResponse_EmptyStopReason(t *testing.T) {
	responseBody := `{
		"content": [
			{"type": "text", "text": "Response"}
		],
		"stop_reason": ""
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	// Empty stop reason should default to EndTurn.
	assert.Equal(t, types.StopReasonEndTurn, result.StopReason)
}

func TestParseBedrockResponse_MissingFields(t *testing.T) {
	responseBody := `{
		"content": [
			{"type": "text", "text": "Response"}
		]
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	assert.Equal(t, "Response", result.Content)
	// Missing stop_reason should default to EndTurn.
	assert.Equal(t, types.StopReasonEndTurn, result.StopReason)
	// Missing usage should be nil.
	assert.Nil(t, result.Usage)
}

func TestParseBedrockResponse_OnlyInputTokens(t *testing.T) {
	responseBody := `{
		"content": [
			{"type": "text", "text": "Response"}
		],
		"stop_reason": "end_turn",
		"usage": {
			"input_tokens": 100,
			"output_tokens": 0
		}
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	require.NotNil(t, result.Usage)
	assert.Equal(t, int64(100), result.Usage.InputTokens)
	assert.Equal(t, int64(0), result.Usage.OutputTokens)
	assert.Equal(t, int64(100), result.Usage.TotalTokens)
}

func TestParseBedrockResponse_OnlyOutputTokens(t *testing.T) {
	responseBody := `{
		"content": [
			{"type": "text", "text": "Response"}
		],
		"stop_reason": "end_turn",
		"usage": {
			"input_tokens": 0,
			"output_tokens": 50
		}
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	require.NotNil(t, result.Usage)
	assert.Equal(t, int64(0), result.Usage.InputTokens)
	assert.Equal(t, int64(50), result.Usage.OutputTokens)
	assert.Equal(t, int64(50), result.Usage.TotalTokens)
}

func TestParseBedrockResponse_MalformedJSON(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "Missing closing brace",
			body: `{"content": [{"type": "text", "text": "test"}`,
		},
		{
			name: "Invalid JSON syntax",
			body: `{content: "test"}`,
		},
		{
			name: "Empty string",
			body: ``,
		},
		{
			name: "Just whitespace",
			body: `   `,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseBedrockResponse([]byte(tt.body))
			assert.Error(t, err)
			assert.Nil(t, result)
		})
	}
}

func TestParseBedrockResponse_ToolUseWithMissingFields(t *testing.T) {
	// Test tool use with missing ID.
	responseBody := `{
		"content": [
			{
				"type": "tool_use",
				"name": "test_tool",
				"input": {"param": "value"}
			}
		],
		"stop_reason": "tool_use"
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	require.Len(t, result.ToolCalls, 1)
	assert.Empty(t, result.ToolCalls[0].ID) // Missing ID should be empty string.
	assert.Equal(t, "test_tool", result.ToolCalls[0].Name)
}

func TestParseBedrockResponse_ToolUseWithMissingName(t *testing.T) {
	// Test tool use with missing name.
	responseBody := `{
		"content": [
			{
				"type": "tool_use",
				"id": "call_123",
				"input": {"param": "value"}
			}
		],
		"stop_reason": "tool_use"
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "call_123", result.ToolCalls[0].ID)
	assert.Empty(t, result.ToolCalls[0].Name) // Missing name should be empty string.
}

func TestParseBedrockResponse_TextWithEmptyString(t *testing.T) {
	responseBody := `{
		"content": [
			{"type": "text", "text": ""},
			{"type": "text", "text": "Non-empty"}
		],
		"stop_reason": "end_turn"
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	// Empty text should still be included.
	assert.Equal(t, "Non-empty", result.Content)
}

func TestParseBedrockResponse_InterleavedTextAndToolUse(t *testing.T) {
	responseBody := `{
		"content": [
			{"type": "text", "text": "Before tool: "},
			{
				"type": "tool_use",
				"id": "call_1",
				"name": "tool1",
				"input": {"a": 1}
			},
			{"type": "text", "text": "Between tools: "},
			{
				"type": "tool_use",
				"id": "call_2",
				"name": "tool2",
				"input": {"b": 2}
			},
			{"type": "text", "text": "After tools."}
		],
		"stop_reason": "tool_use"
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	assert.Equal(t, "Before tool: Between tools: After tools.", result.Content)
	require.Len(t, result.ToolCalls, 2)
	assert.Equal(t, "tool1", result.ToolCalls[0].Name)
	assert.Equal(t, "tool2", result.ToolCalls[1].Name)
}

func TestParseBedrockResponse_ToolInputWithNestedObjects(t *testing.T) {
	responseBody := `{
		"content": [
			{
				"type": "tool_use",
				"id": "call_nested",
				"name": "complex_tool",
				"input": {
					"level1": {
						"level2": {
							"level3": "deep value"
						},
						"array": [1, 2, 3]
					}
				}
			}
		],
		"stop_reason": "tool_use"
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	require.Len(t, result.ToolCalls, 1)

	level1, ok := result.ToolCalls[0].Input["level1"].(map[string]interface{})
	require.True(t, ok)

	level2, ok := level1["level2"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "deep value", level2["level3"])

	array, ok := level1["array"].([]interface{})
	require.True(t, ok)
	assert.Len(t, array, 3)
}

func TestClientGetters_NilConfig(t *testing.T) {
	// This shouldn't happen in practice, but test robustness.
	client := &Client{
		client: nil,
		config: nil,
		region: "us-west-2",
	}

	// Should panic or return zero values if config is nil.
	// Testing that it doesn't crash the program.
	defer func() {
		if r := recover(); r != nil {
			t.Log("Recovered from expected panic:", r)
		}
	}()

	// These will panic with nil config, which is acceptable.
	// Real code should never create a client with nil config.
	_ = client.GetRegion() // This one should work.
	assert.Equal(t, "us-west-2", client.GetRegion())
}

func TestProviderName(t *testing.T) {
	// Verify provider name constant.
	assert.Equal(t, "bedrock", ProviderName)
	// Ensure it's not empty or contains unexpected characters.
	assert.NotEmpty(t, ProviderName)
	assert.NotContains(t, ProviderName, " ")
	assert.NotContains(t, ProviderName, "/")
}

func TestDefaultConstants_Values(t *testing.T) {
	// Verify all default constants have reasonable values.
	assert.Greater(t, DefaultMaxTokens, 0)
	assert.Greater(t, DefaultMaxTokens, 1000) // Should be at least 1000.
	assert.NotEmpty(t, DefaultModel)
	assert.NotEmpty(t, DefaultRegion)
	assert.Contains(t, DefaultModel, "anthropic") // Bedrock uses Anthropic models.
	assert.Contains(t, DefaultRegion, "-")        // AWS regions have hyphens.
}

func TestConvertMessagesToBedrockFormat_AlternatingRoles(t *testing.T) {
	// Test conversation with alternating user and assistant messages.
	messages := []types.Message{
		{Role: types.RoleUser, Content: "Q1"},
		{Role: types.RoleAssistant, Content: "A1"},
		{Role: types.RoleUser, Content: "Q2"},
		{Role: types.RoleAssistant, Content: "A2"},
		{Role: types.RoleUser, Content: "Q3"},
		{Role: types.RoleAssistant, Content: "A3"},
	}

	result := convertMessagesToBedrockFormat(messages)

	require.Len(t, result, 6)
	for i := 0; i < 6; i++ {
		if i%2 == 0 {
			assert.Equal(t, "user", result[i]["role"])
		} else {
			assert.Equal(t, "assistant", result[i]["role"])
		}
	}
}

func TestConvertToolsToBedrockFormat_LargeNumberOfTools(t *testing.T) {
	// Test with many tools.
	var availableTools []tools.Tool
	for i := 0; i < 100; i++ {
		availableTools = append(availableTools, &mockTool{
			name:        "tool_" + string(rune('a'+i%26)),
			description: "Description",
			parameters:  []tools.Parameter{},
		})
	}

	result := convertToolsToBedrockFormat(availableTools)

	assert.Len(t, result, 100)
}

func TestConvertToolsToBedrockFormat_LargeNumberOfParameters(t *testing.T) {
	// Test tool with many parameters.
	var params []tools.Parameter
	for i := 0; i < 20; i++ {
		params = append(params, tools.Parameter{
			Name:        "param_" + string(rune('a'+i)),
			Type:        tools.ParamTypeString,
			Description: "Param description",
			Required:    i%2 == 0,
		})
	}

	availableTools := []tools.Tool{
		&mockTool{
			name:        "many_params_tool",
			description: "Tool with many parameters",
			parameters:  params,
		},
	}

	result := convertToolsToBedrockFormat(availableTools)

	require.Len(t, result, 1)
	inputSchema, ok := result[0]["input_schema"].(map[string]interface{})
	require.True(t, ok)

	properties, ok := inputSchema["properties"].(map[string]interface{})
	require.True(t, ok)
	assert.Len(t, properties, 20)

	required, ok := inputSchema["required"].([]string)
	require.True(t, ok)
	assert.Len(t, required, 10) // Half are required.
}

func TestParseBedrockResponse_AllStopReasons(t *testing.T) {
	// Test all possible stop reasons.
	tests := []struct {
		apiReason      string
		expectedReason types.StopReason
	}{
		{"end_turn", types.StopReasonEndTurn},
		{"tool_use", types.StopReasonToolUse},
		{"max_tokens", types.StopReasonMaxTokens},
		{"stop_sequence", types.StopReasonEndTurn},  // Unknown maps to EndTurn.
		{"content_filter", types.StopReasonEndTurn}, // Unknown maps to EndTurn.
		{"", types.StopReasonEndTurn},               // Empty maps to EndTurn.
	}

	for _, tt := range tests {
		t.Run("StopReason_"+tt.apiReason, func(t *testing.T) {
			responseBody := `{
				"content": [{"type": "text", "text": "test"}],
				"stop_reason": "` + tt.apiReason + `"
			}`

			result, err := parseBedrockResponse([]byte(responseBody))
			require.NoError(t, err)
			assert.Equal(t, tt.expectedReason, result.StopReason)
		})
	}
}

func TestParseBedrockResponse_ZeroCacheTokens(t *testing.T) {
	// Test that cache tokens are always 0 for Bedrock.
	responseBody := `{
		"content": [{"type": "text", "text": "Response"}],
		"stop_reason": "end_turn",
		"usage": {
			"input_tokens": 100,
			"output_tokens": 50
		}
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	require.NotNil(t, result.Usage)
	assert.Equal(t, int64(0), result.Usage.CacheReadTokens)
	assert.Equal(t, int64(0), result.Usage.CacheCreationTokens)
}

func TestRequestBodyMarshaling(t *testing.T) {
	// Test that various request bodies can be marshaled.
	tests := []struct {
		name        string
		requestBody map[string]interface{}
	}{
		{
			name: "Simple message",
			requestBody: map[string]interface{}{
				"anthropic_version": "bedrock-2023-05-31",
				"max_tokens":        4096,
				"messages": []map[string]string{
					{"role": "user", "content": "Hello"},
				},
			},
		},
		{
			name: "With tools",
			requestBody: map[string]interface{}{
				"anthropic_version": "bedrock-2023-05-31",
				"max_tokens":        4096,
				"messages": []map[string]string{
					{"role": "user", "content": "Hello"},
				},
				"tools": []map[string]interface{}{
					{
						"name":        "test_tool",
						"description": "A test",
						"input_schema": map[string]interface{}{
							"type":       "object",
							"properties": map[string]interface{}{},
							"required":   []string{},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)
			assert.NotEmpty(t, bodyBytes)

			// Verify it can be unmarshaled back.
			var decoded map[string]interface{}
			err = json.Unmarshal(bodyBytes, &decoded)
			require.NoError(t, err)
			assert.Equal(t, "bedrock-2023-05-31", decoded["anthropic_version"])
		})
	}
}

func TestExtractConfig_ZeroMaxTokens(t *testing.T) {
	// Test that zero max tokens uses default.
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"bedrock": {
						MaxTokens: 0, // Zero should use default.
					},
				},
			},
		},
	}

	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:     DefaultModel,
		MaxTokens: DefaultMaxTokens,
		BaseURL:   DefaultRegion,
	})

	assert.Equal(t, DefaultMaxTokens, config.MaxTokens)
}

func TestExtractConfig_NegativeMaxTokens(t *testing.T) {
	// Test that negative max tokens uses default (base.ExtractConfig treats 0 and negative as "use default").
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"bedrock": {
						MaxTokens: -100, // Negative should use default.
					},
				},
			},
		},
	}

	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:     DefaultModel,
		MaxTokens: DefaultMaxTokens,
		BaseURL:   DefaultRegion,
	})

	// base.ExtractConfig uses default for values <= 0.
	assert.Equal(t, DefaultMaxTokens, config.MaxTokens)
}

func TestParseBedrockResponse_NumericTypes(t *testing.T) {
	// Test various numeric types in tool inputs.
	responseBody := `{
		"content": [
			{
				"type": "tool_use",
				"id": "call_numeric",
				"name": "numeric_tool",
				"input": {
					"int": 42,
					"float": 3.14,
					"negative": -100,
					"zero": 0,
					"large": 999999999
				}
			}
		],
		"stop_reason": "tool_use"
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, float64(42), result.ToolCalls[0].Input["int"])
	assert.Equal(t, float64(3.14), result.ToolCalls[0].Input["float"])
	assert.Equal(t, float64(-100), result.ToolCalls[0].Input["negative"])
	assert.Equal(t, float64(0), result.ToolCalls[0].Input["zero"])
	assert.Equal(t, float64(999999999), result.ToolCalls[0].Input["large"])
}

func TestParseBedrockResponse_BooleanTypes(t *testing.T) {
	// Test boolean values in tool inputs.
	responseBody := `{
		"content": [
			{
				"type": "tool_use",
				"id": "call_bool",
				"name": "bool_tool",
				"input": {
					"enabled": true,
					"disabled": false
				}
			}
		],
		"stop_reason": "tool_use"
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, true, result.ToolCalls[0].Input["enabled"])
	assert.Equal(t, false, result.ToolCalls[0].Input["disabled"])
}

func TestParseBedrockResponse_ArrayTypes(t *testing.T) {
	// Test array values in tool inputs.
	responseBody := `{
		"content": [
			{
				"type": "tool_use",
				"id": "call_array",
				"name": "array_tool",
				"input": {
					"strings": ["a", "b", "c"],
					"numbers": [1, 2, 3],
					"mixed": ["text", 123, true, null],
					"empty": []
				}
			}
		],
		"stop_reason": "tool_use"
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	require.Len(t, result.ToolCalls, 1)

	strings, ok := result.ToolCalls[0].Input["strings"].([]interface{})
	require.True(t, ok)
	assert.Len(t, strings, 3)

	numbers, ok := result.ToolCalls[0].Input["numbers"].([]interface{})
	require.True(t, ok)
	assert.Len(t, numbers, 3)

	mixed, ok := result.ToolCalls[0].Input["mixed"].([]interface{})
	require.True(t, ok)
	assert.Len(t, mixed, 4)

	empty, ok := result.ToolCalls[0].Input["empty"].([]interface{})
	require.True(t, ok)
	assert.Empty(t, empty)
}

func TestConvertMessagesToBedrockFormat_UnicodeContent(t *testing.T) {
	// Test with Unicode characters.
	messages := []types.Message{
		{Role: types.RoleUser, Content: "Hello 世界 🌍"},
		{Role: types.RoleAssistant, Content: "Привет мир"},
		{Role: types.RoleUser, Content: "مرحبا بالعالم"},
	}

	result := convertMessagesToBedrockFormat(messages)

	require.Len(t, result, 3)
	assert.Equal(t, "Hello 世界 🌍", result[0]["content"])
	assert.Equal(t, "Привет мир", result[1]["content"])
	assert.Equal(t, "مرحبا بالعالم", result[2]["content"])
}

func TestConvertToolsToBedrockFormat_UnicodeInToolNames(t *testing.T) {
	// Test that tool names with special characters work.
	availableTools := []tools.Tool{
		&mockTool{
			name:        "test_tool_123",
			description: "Description with émojis 🎉 and spëcial çharacters",
			parameters: []tools.Parameter{
				{Name: "param_α", Type: tools.ParamTypeString, Description: "Greek letter", Required: true},
			},
		},
	}

	result := convertToolsToBedrockFormat(availableTools)

	require.Len(t, result, 1)
	assert.Equal(t, "test_tool_123", result[0]["name"])
	assert.Contains(t, result[0]["description"], "émojis")
}

func TestParseBedrockResponse_ContentTypeCase(t *testing.T) {
	// Test that content type matching is case-sensitive (as it should be).
	responseBody := `{
		"content": [
			{"type": "text", "text": "lowercase"},
			{"type": "TEXT", "text": "uppercase"},
			{"type": "Text", "text": "capitalized"}
		],
		"stop_reason": "end_turn"
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	// Only "text" (lowercase) should match.
	assert.Equal(t, "lowercase", result.Content)
}

func TestParseBedrockResponse_VeryLargeTokenCounts(t *testing.T) {
	// Test with very large token counts.
	responseBody := `{
		"content": [{"type": "text", "text": "Response"}],
		"stop_reason": "end_turn",
		"usage": {
			"input_tokens": 9223372036854775807,
			"output_tokens": 9223372036854775807
		}
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	require.NotNil(t, result.Usage)
	// Should handle max int64 values.
	assert.Equal(t, int64(9223372036854775807), result.Usage.InputTokens)
}

func TestClientGetters_VariousModels(t *testing.T) {
	// Test getters with various model configurations.
	models := []string{
		DefaultModel,
		"anthropic.claude-3-haiku-20240307-v1:0",
		"anthropic.claude-3-opus-20240229-v1:0",
		"anthropic.claude-3-sonnet-20240229-v1:0",
		"anthropic.claude-instant-v1",
		"anthropic.claude-v2",
	}

	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			client := &Client{
				config: &base.Config{
					Model: model,
				},
				region: DefaultRegion,
			}

			assert.Equal(t, model, client.GetModel())
		})
	}
}

func TestExtractConfig_EmptyModel(t *testing.T) {
	// Test that empty model uses default.
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"bedrock": {
						Model: "", // Empty model.
					},
				},
			},
		},
	}

	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:     DefaultModel,
		MaxTokens: DefaultMaxTokens,
		BaseURL:   DefaultRegion,
	})

	assert.Equal(t, DefaultModel, config.Model)
}

func TestExtractConfig_WhitespaceModel(t *testing.T) {
	// Test that whitespace-only model uses default.
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"bedrock": {
						Model: "   ", // Whitespace only.
					},
				},
			},
		},
	}

	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:     DefaultModel,
		MaxTokens: DefaultMaxTokens,
		BaseURL:   DefaultRegion,
	})

	// base.ExtractConfig treats empty/whitespace as "use the value provided".
	assert.Equal(t, "   ", config.Model)
}

func TestConvertMessagesToBedrockFormat_VeryLongConversation(t *testing.T) {
	// Test with a long conversation history.
	var messages []types.Message
	for i := 0; i < 1000; i++ {
		role := types.RoleUser
		if i%2 == 1 {
			role = types.RoleAssistant
		}
		messages = append(messages, types.Message{
			Role:    role,
			Content: "Message " + string(rune('0'+i%10)),
		})
	}

	result := convertMessagesToBedrockFormat(messages)

	assert.Len(t, result, 1000)
}

func TestParseBedrockResponse_EmptyToolID(t *testing.T) {
	// Test tool use with empty ID string.
	responseBody := `{
		"content": [
			{
				"type": "tool_use",
				"id": "",
				"name": "test_tool",
				"input": {}
			}
		],
		"stop_reason": "tool_use"
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	require.Len(t, result.ToolCalls, 1)
	assert.Empty(t, result.ToolCalls[0].ID)
}

func TestParseBedrockResponse_EmptyToolName(t *testing.T) {
	// Test tool use with empty name string.
	responseBody := `{
		"content": [
			{
				"type": "tool_use",
				"id": "call_123",
				"name": "",
				"input": {}
			}
		],
		"stop_reason": "tool_use"
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	require.Len(t, result.ToolCalls, 1)
	assert.Empty(t, result.ToolCalls[0].Name)
}

func TestConvertToolsToBedrockFormat_EmptyToolName(t *testing.T) {
	// Test tool with empty name.
	availableTools := []tools.Tool{
		&mockTool{
			name:        "",
			description: "Tool with empty name",
			parameters:  []tools.Parameter{},
		},
	}

	result := convertToolsToBedrockFormat(availableTools)

	require.Len(t, result, 1)
	assert.Empty(t, result[0]["name"])
}

func TestConvertToolsToBedrockFormat_EmptyDescription(t *testing.T) {
	// Test tool with empty description.
	availableTools := []tools.Tool{
		&mockTool{
			name:        "test_tool",
			description: "",
			parameters:  []tools.Parameter{},
		},
	}

	result := convertToolsToBedrockFormat(availableTools)

	require.Len(t, result, 1)
	assert.Empty(t, result[0]["description"])
}

func TestParseBedrockResponse_ContentWithNullText(t *testing.T) {
	// Test response where text field is null (should be handled gracefully).
	responseBody := `{
		"content": [
			{"type": "text"}
		],
		"stop_reason": "end_turn"
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	// Missing text field should result in empty string.
	assert.Empty(t, result.Content)
}

func TestRequestBodyMarshaling_SpecialCharacters(t *testing.T) {
	// Test marshaling with special characters that need escaping.
	requestBody := map[string]interface{}{
		"anthropic_version": "bedrock-2023-05-31",
		"max_tokens":        4096,
		"messages": []map[string]string{
			{"role": "user", "content": "Test with \"quotes\" and \n newlines \t tabs"},
		},
	}

	bodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)
	assert.NotEmpty(t, bodyBytes)

	// Verify it round-trips correctly.
	var decoded map[string]interface{}
	err = json.Unmarshal(bodyBytes, &decoded)
	require.NoError(t, err)

	messages := decoded["messages"].([]interface{})
	firstMessage := messages[0].(map[string]interface{})
	assert.Contains(t, firstMessage["content"], "\"quotes\"")
}

func TestExtractConfig_MultipleProviders(t *testing.T) {
	// Test that bedrock config is extracted correctly when multiple providers exist.
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"openai": {
						Model:     "gpt-4o",
						MaxTokens: 2000,
					},
					"bedrock": {
						Model:     "anthropic.claude-3-haiku-20240307-v1:0",
						MaxTokens: 8000,
						BaseURL:   "ap-southeast-1",
					},
					"anthropic": {
						Model:     "claude-3-opus-20240229",
						MaxTokens: 1000,
					},
				},
			},
		},
	}

	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:     DefaultModel,
		MaxTokens: DefaultMaxTokens,
		BaseURL:   DefaultRegion,
	})

	// Should extract only bedrock config.
	assert.Equal(t, "anthropic.claude-3-haiku-20240307-v1:0", config.Model)
	assert.Equal(t, 8000, config.MaxTokens)
	assert.Equal(t, "ap-southeast-1", config.BaseURL)
}
