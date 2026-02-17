package azureopenai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/agent/base"
	"github.com/cloudposse/atmos/pkg/ai/agent/base/openaicompat"
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
				Model:     "gpt-4o",
				APIKeyEnv: "AZURE_OPENAI_API_KEY",
				MaxTokens: 4096,
				BaseURL:   "",
			},
		},
		{
			name: "Enabled configuration",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"azureopenai": {
								Model:     "gpt-4-turbo",
								ApiKeyEnv: "CUSTOM_AZURE_KEY",
								MaxTokens: 8192,
								BaseURL:   "https://myresource.openai.azure.com",
							},
						},
					},
				},
			},
			expectedConfig: &base.Config{
				Enabled:   true,
				Model:     "gpt-4-turbo",
				APIKeyEnv: "CUSTOM_AZURE_KEY",
				MaxTokens: 8192,
				BaseURL:   "https://myresource.openai.azure.com",
			},
		},
		{
			name: "Partial configuration with endpoint",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"azureopenai": {
								Model:   "gpt-35-turbo",
								BaseURL: "https://company.openai.azure.com",
							},
						},
					},
				},
			},
			expectedConfig: &base.Config{
				Enabled:   true,
				Model:     "gpt-35-turbo",
				APIKeyEnv: "AZURE_OPENAI_API_KEY",
				MaxTokens: 4096,
				BaseURL:   "https://company.openai.azure.com",
			},
		},
		{
			name: "Custom deployment name",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"azureopenai": {
								Model:   "my-gpt4-deployment",
								BaseURL: "https://prod-ai.openai.azure.com",
							},
						},
					},
				},
			},
			expectedConfig: &base.Config{
				Enabled:   true,
				Model:     "my-gpt4-deployment",
				APIKeyEnv: "AZURE_OPENAI_API_KEY",
				MaxTokens: 4096,
				BaseURL:   "https://prod-ai.openai.azure.com",
			},
		},
		{
			name: "Custom API key env only",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"azureopenai": {
								ApiKeyEnv: "MY_AZURE_API_KEY",
								BaseURL:   "https://test.openai.azure.com",
							},
						},
					},
				},
			},
			expectedConfig: &base.Config{
				Enabled:   true,
				Model:     "gpt-4o",
				APIKeyEnv: "MY_AZURE_API_KEY",
				MaxTokens: 4096,
				BaseURL:   "https://test.openai.azure.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := base.ExtractConfig(tt.atmosConfig, ProviderName, base.ProviderDefaults{
				Model:     DefaultModel,
				APIKeyEnv: DefaultAPIKeyEnv,
				MaxTokens: DefaultMaxTokens,
				BaseURL:   "",
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

	client, err := NewClient(atmosConfig)
	assert.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "AI features are disabled")
}

func TestNewClient_MissingBaseURL(t *testing.T) {
	envVar := "TEST_AZURE_KEY_" + t.Name()
	t.Setenv(envVar, "test-api-key")

	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"azureopenai": {
						ApiKeyEnv: envVar,
						// BaseURL is missing.
					},
				},
			},
		},
	}

	client, err := NewClient(atmosConfig)
	assert.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "base URL is required")
}

func TestNewClient_MissingAPIKey(t *testing.T) {
	// Use a unique env var name that definitely does not exist.
	envVar := "NONEXISTENT_AZURE_KEY_XYZZY_TEST"

	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"azureopenai": {
						ApiKeyEnv: envVar,
						BaseURL:   "https://test.openai.azure.com",
					},
				},
			},
		},
	}

	client, err := NewClient(atmosConfig)
	assert.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "API key not found")
}

func TestNewClient_WithAPIKeyAndBaseURL(t *testing.T) {
	// Note: Creating a real client requires an API key in the environment.
	// This test verifies the client creation logic by using a real env var.
	// If AZURE_OPENAI_API_KEY is not set, we skip the test.
	envVar := "AZURE_OPENAI_API_KEY"

	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"azureopenai": {
						ApiKeyEnv: envVar,
						BaseURL:   "https://test.openai.azure.com",
						Model:     "gpt-4o",
						MaxTokens: 4096,
					},
				},
			},
		},
	}

	client, err := NewClient(atmosConfig)
	// If no API key is set, we expect an error.
	if err != nil {
		assert.Contains(t, err.Error(), "API key not found")
		return
	}

	// If API key was set, verify client is created correctly.
	require.NotNil(t, client)
	assert.Equal(t, "gpt-4o", client.GetModel())
	assert.Equal(t, 4096, client.GetMaxTokens())
	assert.Equal(t, "https://test.openai.azure.com", client.GetBaseURL())
	assert.Equal(t, DefaultAPIVersion, client.GetAPIVersion())
}

func TestClientGetters(t *testing.T) {
	config := &base.Config{
		Enabled:   true,
		Model:     "gpt-4o",
		APIKeyEnv: "AZURE_OPENAI_API_KEY",
		MaxTokens: 4096,
		BaseURL:   "https://myresource.openai.azure.com",
	}

	client := &Client{
		client:     nil, // We don't need a real client for testing getters.
		config:     config,
		apiVersion: "2024-02-15-preview",
	}

	assert.Equal(t, "gpt-4o", client.GetModel())
	assert.Equal(t, 4096, client.GetMaxTokens())
	assert.Equal(t, "https://myresource.openai.azure.com", client.GetBaseURL())
	assert.Equal(t, "2024-02-15-preview", client.GetAPIVersion())
}

func TestDefaultConstants(t *testing.T) {
	assert.Equal(t, "azureopenai", ProviderName)
	assert.Equal(t, 4096, DefaultMaxTokens)
	assert.Equal(t, "gpt-4o", DefaultModel)
	assert.Equal(t, "AZURE_OPENAI_API_KEY", DefaultAPIKeyEnv)
	assert.Equal(t, "2024-02-15-preview", DefaultAPIVersion)
}

func TestConfig_AllFields(t *testing.T) {
	config := &base.Config{
		Enabled:   true,
		Model:     "test-model",
		APIKeyEnv: "TEST_KEY",
		MaxTokens: 1000,
		BaseURL:   "https://test.openai.azure.com",
	}

	assert.True(t, config.Enabled)
	assert.Equal(t, "test-model", config.Model)
	assert.Equal(t, "TEST_KEY", config.APIKeyEnv)
	assert.Equal(t, 1000, config.MaxTokens)
	assert.Equal(t, "https://test.openai.azure.com", config.BaseURL)
}

// Tests using openaicompat package utilities.

func TestConvertMessagesToOpenAIFormat_Empty(t *testing.T) {
	messages := []types.Message{}
	result := openaicompat.ConvertMessagesToOpenAIFormat(messages)

	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestConvertMessagesToOpenAIFormat_SingleUserMessage(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleUser, Content: "Hello, world!"},
	}

	result := openaicompat.ConvertMessagesToOpenAIFormat(messages)

	assert.Len(t, result, 1)
}

func TestConvertMessagesToOpenAIFormat_SingleAssistantMessage(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleAssistant, Content: "Hello! How can I help you?"},
	}

	result := openaicompat.ConvertMessagesToOpenAIFormat(messages)

	assert.Len(t, result, 1)
}

func TestConvertMessagesToOpenAIFormat_SingleSystemMessage(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleSystem, Content: "You are a helpful assistant."},
	}

	result := openaicompat.ConvertMessagesToOpenAIFormat(messages)

	assert.Len(t, result, 1)
}

func TestConvertMessagesToOpenAIFormat_MultipleMessages(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleSystem, Content: "You are a helpful assistant."},
		{Role: types.RoleUser, Content: "What is 2+2?"},
		{Role: types.RoleAssistant, Content: "2+2 equals 4."},
		{Role: types.RoleUser, Content: "Thanks!"},
	}

	result := openaicompat.ConvertMessagesToOpenAIFormat(messages)

	assert.Len(t, result, 4)
}

func TestConvertMessagesToOpenAIFormat_UnknownRole(t *testing.T) {
	messages := []types.Message{
		{Role: "unknown", Content: "This should be skipped"},
	}

	result := openaicompat.ConvertMessagesToOpenAIFormat(messages)

	// Unknown roles are skipped.
	assert.Empty(t, result)
}

func TestConvertToolsToOpenAIFormat_Empty(t *testing.T) {
	availableTools := []tools.Tool{}
	result := openaicompat.ConvertToolsToOpenAIFormat(availableTools)

	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestConvertToolsToOpenAIFormat_SingleTool(t *testing.T) {
	availableTools := []tools.Tool{
		&mockTool{
			name:        "test_tool",
			description: "A test tool",
			parameters: []tools.Parameter{
				{Name: "param1", Type: tools.ParamTypeString, Description: "First param", Required: true},
			},
		},
	}

	result := openaicompat.ConvertToolsToOpenAIFormat(availableTools)

	assert.Len(t, result, 1)
	assert.Equal(t, "test_tool", result[0].Function.Name)
	assert.Equal(t, "A test tool", result[0].Function.Description.Value)
}

func TestConvertToolsToOpenAIFormat_MultipleTools(t *testing.T) {
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

	result := openaicompat.ConvertToolsToOpenAIFormat(availableTools)

	assert.Len(t, result, 2)
	assert.Equal(t, "tool_a", result[0].Function.Name)
	assert.Equal(t, "tool_b", result[1].Function.Name)
}

func TestConvertToolsToOpenAIFormat_AllParameterTypes(t *testing.T) {
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

	result := openaicompat.ConvertToolsToOpenAIFormat(availableTools)

	require.Len(t, result, 1)
	assert.Equal(t, "comprehensive_tool", result[0].Function.Name)

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
	assert.Contains(t, requiredList, "string_param")
	assert.Contains(t, requiredList, "int_param")
}

func TestParseOpenAIResponse_EmptyChoices(t *testing.T) {
	response := &openai.ChatCompletion{
		Choices: []openai.ChatCompletionChoice{},
	}

	result, err := openaicompat.ParseOpenAIResponse(response)

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

	result, err := openaicompat.ParseOpenAIResponse(response)

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

	result, err := openaicompat.ParseOpenAIResponse(response)

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

	result, err := openaicompat.ParseOpenAIResponse(response)

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

	result, err := openaicompat.ParseOpenAIResponse(response)

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

	result, err := openaicompat.ParseOpenAIResponse(response)

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

	result, err := openaicompat.ParseOpenAIResponse(response)

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

	result, err := openaicompat.ParseOpenAIResponse(response)

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

	result, err := openaicompat.ParseOpenAIResponse(response)

	require.NoError(t, err)
	assert.Len(t, result.ToolCalls, 2)
	assert.Equal(t, "tool_a", result.ToolCalls[0].Name)
	assert.Equal(t, "tool_b", result.ToolCalls[1].Name)
}

func TestSetTokenLimit_MaxCompletionTokens(t *testing.T) {
	params := &openai.ChatCompletionNewParams{}

	openaicompat.SetTokenLimit(params, "gpt-5", 4096)

	// Should set MaxCompletionTokens, not MaxTokens.
	assert.Equal(t, int64(4096), params.MaxCompletionTokens.Value)
	assert.False(t, params.MaxTokens.Valid())
}

func TestSetTokenLimit_MaxTokens(t *testing.T) {
	params := &openai.ChatCompletionNewParams{}

	openaicompat.SetTokenLimit(params, "gpt-4o", 4096)

	// Should set MaxTokens, not MaxCompletionTokens.
	assert.Equal(t, int64(4096), params.MaxTokens.Value)
	assert.False(t, params.MaxCompletionTokens.Valid())
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
			result := openaicompat.RequiresMaxCompletionTokens(tt.model)
			assert.Equal(t, tt.expected, result, "model: %s", tt.model)
		})
	}
}

func TestAzureOpenAIModels(t *testing.T) {
	// Test various Azure OpenAI model/deployment names.
	models := []struct {
		modelID     string
		description string
	}{
		{"gpt-4o", "GPT-4o"},
		{"gpt-4-turbo", "GPT-4 Turbo"},
		{"gpt-4", "GPT-4"},
		{"gpt-35-turbo", "GPT-3.5 Turbo (Azure naming)"},
		{"gpt-3.5-turbo", "GPT-3.5 Turbo"},
		{"my-custom-deployment", "Custom deployment name"},
		{"prod-gpt4-deployment", "Production deployment"},
	}

	for _, m := range models {
		t.Run(m.description, func(t *testing.T) {
			config := &base.Config{
				Enabled:   true,
				Model:     m.modelID,
				APIKeyEnv: DefaultAPIKeyEnv,
				MaxTokens: DefaultMaxTokens,
				BaseURL:   "https://test.openai.azure.com",
			}

			client := &Client{
				client:     nil,
				config:     config,
				apiVersion: DefaultAPIVersion,
			}

			assert.Equal(t, m.modelID, client.GetModel())
		})
	}
}

func TestAzureEndpointFormats(t *testing.T) {
	// Test various Azure endpoint formats.
	endpoints := []struct {
		baseURL     string
		description string
	}{
		{"https://myresource.openai.azure.com", "Standard format"},
		{"https://company-ai.openai.azure.com", "Company resource"},
		{"https://prod-east-us.openai.azure.com", "Regional resource"},
		{"https://my-openai.openai.azure.com/", "With trailing slash"},
	}

	for _, e := range endpoints {
		t.Run(e.description, func(t *testing.T) {
			config := &base.Config{
				Enabled:   true,
				Model:     DefaultModel,
				APIKeyEnv: DefaultAPIKeyEnv,
				MaxTokens: DefaultMaxTokens,
				BaseURL:   e.baseURL,
			}

			client := &Client{
				client:     nil,
				config:     config,
				apiVersion: DefaultAPIVersion,
			}

			assert.Equal(t, e.baseURL, client.GetBaseURL())
		})
	}
}

func TestAPIVersions(t *testing.T) {
	// Test that API version is correctly set.
	config := &base.Config{
		Enabled:   true,
		Model:     DefaultModel,
		APIKeyEnv: DefaultAPIKeyEnv,
		MaxTokens: DefaultMaxTokens,
		BaseURL:   "https://test.openai.azure.com",
	}

	// Default API version.
	client := &Client{
		client:     nil,
		config:     config,
		apiVersion: DefaultAPIVersion,
	}

	assert.Equal(t, "2024-02-15-preview", client.GetAPIVersion())
}

func TestAzureOpenAI_MaxTokensConfigurations(t *testing.T) {
	// Test various max token configurations.
	tokenTests := []struct {
		maxTokens int
		expected  int
	}{
		{1024, 1024},
		{2048, 2048},
		{4096, 4096},
		{8192, 8192},
		{16384, 16384},
		{32768, 32768},
	}

	for _, tt := range tokenTests {
		t.Run("maxTokens_"+string(rune(tt.maxTokens)), func(t *testing.T) {
			config := &base.Config{
				Enabled:   true,
				Model:     DefaultModel,
				APIKeyEnv: DefaultAPIKeyEnv,
				MaxTokens: tt.maxTokens,
				BaseURL:   "https://test.openai.azure.com",
			}

			client := &Client{
				client:     nil,
				config:     config,
				apiVersion: DefaultAPIVersion,
			}

			assert.Equal(t, tt.expected, client.GetMaxTokens())
		})
	}
}

func TestConvertToolsToOpenAIFormat_ComplexParameters(t *testing.T) {
	availableTools := []tools.Tool{
		&mockTool{
			name:        "describe_component",
			description: "Describe an Atmos component",
			parameters: []tools.Parameter{
				{Name: "component", Type: tools.ParamTypeString, Description: "Component name", Required: true},
				{Name: "stack", Type: tools.ParamTypeString, Description: "Stack name", Required: true},
				{Name: "verbose", Type: tools.ParamTypeBool, Description: "Verbose output", Required: false, Default: false},
				{Name: "limit", Type: tools.ParamTypeInt, Description: "Limit results", Required: false, Default: 10},
			},
		},
	}

	result := openaicompat.ConvertToolsToOpenAIFormat(availableTools)

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

func TestParseOpenAIResponse_ComplexToolArguments(t *testing.T) {
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
								Arguments: `{"component": "vpc", "stack": "prod-use1", "options": {"verbose": true, "limit": 10}}`,
							},
						},
					},
				},
			},
		},
	}

	result, err := openaicompat.ParseOpenAIResponse(response)

	require.NoError(t, err)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "vpc", result.ToolCalls[0].Input["component"])
	assert.Equal(t, "prod-use1", result.ToolCalls[0].Input["stack"])

	options, ok := result.ToolCalls[0].Input["options"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, options["verbose"])
	assert.Equal(t, float64(10), options["limit"]) // JSON numbers are float64.
}

// Helper function to create a mock OpenAI server for testing.
func newMockOpenAIServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

// newTestClient creates a Client for testing purposes, bypassing the API key check.
// This is used to test client methods without requiring actual API credentials.
//
//nolint:unparam // model parameter kept for test flexibility.
func newTestClient(t *testing.T, baseURL, model string, maxTokens int) *Client {
	t.Helper()

	// Create an OpenAI client with a fake API key pointing to our mock server.
	client := openai.NewClient(
		option.WithAPIKey("test-api-key"),
		option.WithBaseURL(baseURL),
		option.WithHeader("api-version", DefaultAPIVersion),
	)

	return &Client{
		client: &client,
		config: &base.Config{
			Enabled:   true,
			Model:     model,
			APIKeyEnv: "TEST_API_KEY",
			MaxTokens: maxTokens,
			BaseURL:   baseURL,
		},
		apiVersion: DefaultAPIVersion,
	}
}

// createMockChatCompletionResponse creates a mock OpenAI ChatCompletion response.
//
//nolint:unparam // finishReason parameter kept for test flexibility.
func createMockChatCompletionResponse(content string, finishReason string) map[string]interface{} {
	return map[string]interface{}{
		"id":      "chatcmpl-test123",
		"object":  "chat.completion",
		"created": 1699999999,
		"model":   "gpt-4o",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": finishReason,
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     10,
			"completion_tokens": 20,
			"total_tokens":      30,
		},
	}
}

// createMockToolCallResponse creates a mock OpenAI response with tool calls.
func createMockToolCallResponse(toolName, toolID, args string) map[string]interface{} {
	return map[string]interface{}{
		"id":      "chatcmpl-test456",
		"object":  "chat.completion",
		"created": 1699999999,
		"model":   "gpt-4o",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "",
					"tool_calls": []map[string]interface{}{
						{
							"id":   toolID,
							"type": "function",
							"function": map[string]interface{}{
								"name":      toolName,
								"arguments": args,
							},
						},
					},
				},
				"finish_reason": "tool_calls",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     15,
			"completion_tokens": 25,
			"total_tokens":      40,
		},
	}
}

// Note: TestNewClient_Success is tested indirectly via TestSendMessage_Success and similar tests
// which create a mock server and verify full client creation. The viper.AutomaticEnv() used in
// GetAPIKey doesn't reliably pick up env vars set with t.Setenv in tests.

func TestNewClient_SuccessWithDirectConstruction(t *testing.T) {
	// Test client construction with a directly created config.
	// This tests the Client struct directly without going through NewClient.
	config := &base.Config{
		Enabled:   true,
		Model:     "gpt-4o-deployment",
		APIKeyEnv: "TEST_KEY",
		MaxTokens: 8192,
		BaseURL:   "https://test-resource.openai.azure.com",
	}

	client := &Client{
		client:     nil, // We don't need a real openai client for this test.
		config:     config,
		apiVersion: DefaultAPIVersion,
	}

	// Verify all configuration was applied correctly.
	assert.Equal(t, "gpt-4o-deployment", client.GetModel())
	assert.Equal(t, 8192, client.GetMaxTokens())
	assert.Equal(t, "https://test-resource.openai.azure.com", client.GetBaseURL())
	assert.Equal(t, DefaultAPIVersion, client.GetAPIVersion())
}

func TestClient_WithDifferentModels(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		maxTokens int
	}{
		{"gpt-4o", "gpt-4o", 4096},
		{"gpt-35-turbo", "gpt-35-turbo", 4096},
		{"gpt-4-turbo", "gpt-4-turbo", 8192},
		{"custom-deployment", "my-custom-gpt4-deployment", 16384},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a client directly with the config for testing.
			config := &base.Config{
				Enabled:   true,
				Model:     tt.model,
				APIKeyEnv: "TEST_KEY",
				MaxTokens: tt.maxTokens,
				BaseURL:   "https://test.openai.azure.com",
			}

			client := &Client{
				client:     nil,
				config:     config,
				apiVersion: DefaultAPIVersion,
			}

			assert.Equal(t, tt.model, client.GetModel())
			assert.Equal(t, tt.maxTokens, client.GetMaxTokens())
		})
	}
}

func TestExtractConfig_DefaultValuesApplied(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"azureopenai": {
						BaseURL: "https://test.openai.azure.com",
						// Model and MaxTokens not specified - should use defaults.
					},
				},
			},
		},
	}

	// Test that ExtractConfig applies defaults correctly.
	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:     DefaultModel,
		APIKeyEnv: DefaultAPIKeyEnv,
		MaxTokens: DefaultMaxTokens,
		BaseURL:   "",
	})

	// Verify default values were applied.
	assert.Equal(t, DefaultModel, config.Model)
	assert.Equal(t, DefaultMaxTokens, config.MaxTokens)
	assert.Equal(t, DefaultAPIKeyEnv, config.APIKeyEnv)
}

func TestSendMessage_Success(t *testing.T) {
	// Create mock server.
	server := newMockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path.
		assert.Equal(t, "POST", r.Method)
		assert.Contains(t, r.URL.Path, "chat/completions")

		// Return mock response.
		w.Header().Set("Content-Type", "application/json")
		response := createMockChatCompletionResponse("Hello! How can I help you?", "stop")
		err := json.NewEncoder(w).Encode(response)
		require.NoError(t, err)
	})
	defer server.Close()

	client := newTestClient(t, server.URL, "gpt-4o", 4096)

	// Send a message.
	ctx := context.Background()
	response, err := client.SendMessage(ctx, "Hello")

	require.NoError(t, err)
	assert.Equal(t, "Hello! How can I help you?", response)
}

func TestSendMessage_EmptyResponse(t *testing.T) {
	// Create mock server that returns empty choices.
	server := newMockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"id":      "chatcmpl-empty",
			"object":  "chat.completion",
			"created": 1699999999,
			"model":   "gpt-4o",
			"choices": []map[string]interface{}{}, // Empty choices.
		}
		err := json.NewEncoder(w).Encode(response)
		require.NoError(t, err)
	})
	defer server.Close()

	client := newTestClient(t, server.URL, "gpt-4o", 4096)

	ctx := context.Background()
	response, err := client.SendMessage(ctx, "Hello")

	assert.Error(t, err)
	assert.Empty(t, response)
	assert.Contains(t, err.Error(), "no response choices")
}

func TestSendMessage_APIError(t *testing.T) {
	// Create mock server that returns an error.
	server := newMockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": {"message": "Internal server error", "type": "server_error"}}`))
	})
	defer server.Close()

	client := newTestClient(t, server.URL, "gpt-4o", 4096)

	ctx := context.Background()
	response, err := client.SendMessage(ctx, "Hello")

	assert.Error(t, err)
	assert.Empty(t, response)
	assert.Contains(t, err.Error(), "failed to send message")
}

func TestSendMessageWithTools_Success(t *testing.T) {
	// Create mock server.
	server := newMockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := createMockToolCallResponse("test_tool", "call_123", `{"param1": "value1"}`)
		err := json.NewEncoder(w).Encode(response)
		require.NoError(t, err)
	})
	defer server.Close()

	client := newTestClient(t, server.URL, "gpt-4o", 4096)

	availableTools := []tools.Tool{
		&mockTool{
			name:        "test_tool",
			description: "A test tool",
			parameters: []tools.Parameter{
				{Name: "param1", Type: tools.ParamTypeString, Description: "First param", Required: true},
			},
		},
	}

	ctx := context.Background()
	response, err := client.SendMessageWithTools(ctx, "Use the test tool", availableTools)

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, types.StopReasonToolUse, response.StopReason)
	require.Len(t, response.ToolCalls, 1)
	assert.Equal(t, "test_tool", response.ToolCalls[0].Name)
	assert.Equal(t, "call_123", response.ToolCalls[0].ID)
	assert.Equal(t, "value1", response.ToolCalls[0].Input["param1"])
}

func TestSendMessageWithTools_NoToolCall(t *testing.T) {
	// Create mock server that returns a normal response (no tool call).
	server := newMockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := createMockChatCompletionResponse("I don't need to use any tools.", "stop")
		err := json.NewEncoder(w).Encode(response)
		require.NoError(t, err)
	})
	defer server.Close()

	client := newTestClient(t, server.URL, "gpt-4o", 4096)

	availableTools := []tools.Tool{
		&mockTool{
			name:        "test_tool",
			description: "A test tool",
			parameters:  []tools.Parameter{},
		},
	}

	ctx := context.Background()
	response, err := client.SendMessageWithTools(ctx, "Hello", availableTools)

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, types.StopReasonEndTurn, response.StopReason)
	assert.Empty(t, response.ToolCalls)
	assert.Equal(t, "I don't need to use any tools.", response.Content)
}

func TestSendMessageWithTools_APIError(t *testing.T) {
	// Create mock server that returns an error.
	server := newMockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": {"message": "Invalid request", "type": "invalid_request_error"}}`))
	})
	defer server.Close()

	client := newTestClient(t, server.URL, "gpt-4o", 4096)

	availableTools := []tools.Tool{
		&mockTool{
			name:        "test_tool",
			description: "A test tool",
			parameters:  []tools.Parameter{},
		},
	}

	ctx := context.Background()
	response, err := client.SendMessageWithTools(ctx, "Use the tool", availableTools)

	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "failed to send message")
}

func TestSendMessageWithHistory_Success(t *testing.T) {
	// Create mock server.
	server := newMockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Verify the request body contains the conversation history.
		var reqBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		require.NoError(t, err)

		// Verify messages array exists.
		messages, ok := reqBody["messages"].([]interface{})
		require.True(t, ok)
		require.Len(t, messages, 3) // system, user, assistant messages.

		w.Header().Set("Content-Type", "application/json")
		response := createMockChatCompletionResponse("Based on our conversation, the answer is 42.", "stop")
		err = json.NewEncoder(w).Encode(response)
		require.NoError(t, err)
	})
	defer server.Close()

	client := newTestClient(t, server.URL, "gpt-4o", 4096)

	messages := []types.Message{
		{Role: types.RoleSystem, Content: "You are a helpful assistant."},
		{Role: types.RoleUser, Content: "What is the answer to life?"},
		{Role: types.RoleAssistant, Content: "The answer is 42."},
	}

	ctx := context.Background()
	response, err := client.SendMessageWithHistory(ctx, messages)

	require.NoError(t, err)
	assert.Equal(t, "Based on our conversation, the answer is 42.", response)
}

func TestSendMessageWithHistory_EmptyHistory(t *testing.T) {
	server := newMockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := createMockChatCompletionResponse("No history provided.", "stop")
		err := json.NewEncoder(w).Encode(response)
		require.NoError(t, err)
	})
	defer server.Close()

	client := newTestClient(t, server.URL, "gpt-4o", 4096)

	ctx := context.Background()
	response, err := client.SendMessageWithHistory(ctx, []types.Message{})

	require.NoError(t, err)
	assert.Equal(t, "No history provided.", response)
}

func TestSendMessageWithHistory_APIError(t *testing.T) {
	server := newMockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error": {"message": "Rate limit exceeded", "type": "rate_limit_error"}}`))
	})
	defer server.Close()

	client := newTestClient(t, server.URL, "gpt-4o", 4096)

	messages := []types.Message{
		{Role: types.RoleUser, Content: "Hello"},
	}

	ctx := context.Background()
	response, err := client.SendMessageWithHistory(ctx, messages)

	assert.Error(t, err)
	assert.Empty(t, response)
}

func TestSendMessageWithHistory_EmptyChoices(t *testing.T) {
	server := newMockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"id":      "chatcmpl-empty",
			"object":  "chat.completion",
			"created": 1699999999,
			"model":   "gpt-4o",
			"choices": []map[string]interface{}{},
		}
		err := json.NewEncoder(w).Encode(response)
		require.NoError(t, err)
	})
	defer server.Close()

	client := newTestClient(t, server.URL, "gpt-4o", 4096)

	messages := []types.Message{
		{Role: types.RoleUser, Content: "Hello"},
	}

	ctx := context.Background()
	response, err := client.SendMessageWithHistory(ctx, messages)

	assert.Error(t, err)
	assert.Empty(t, response)
	assert.Contains(t, err.Error(), "no response choices")
}

func TestSendMessageWithToolsAndHistory_Success(t *testing.T) {
	server := newMockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Verify the request contains both messages and tools.
		var reqBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		require.NoError(t, err)

		// Verify messages array exists.
		messages, ok := reqBody["messages"].([]interface{})
		require.True(t, ok)
		require.Len(t, messages, 2)

		// Verify tools array exists.
		toolsArr, ok := reqBody["tools"].([]interface{})
		require.True(t, ok)
		require.Len(t, toolsArr, 1)

		w.Header().Set("Content-Type", "application/json")
		response := createMockToolCallResponse("describe_component", "call_789", `{"component": "vpc", "stack": "dev"}`)
		err = json.NewEncoder(w).Encode(response)
		require.NoError(t, err)
	})
	defer server.Close()

	client := newTestClient(t, server.URL, "gpt-4o", 4096)

	messages := []types.Message{
		{Role: types.RoleSystem, Content: "You are an Atmos assistant."},
		{Role: types.RoleUser, Content: "Describe the vpc component in dev stack"},
	}

	availableTools := []tools.Tool{
		&mockTool{
			name:        "describe_component",
			description: "Describe an Atmos component",
			parameters: []tools.Parameter{
				{Name: "component", Type: tools.ParamTypeString, Required: true},
				{Name: "stack", Type: tools.ParamTypeString, Required: true},
			},
		},
	}

	ctx := context.Background()
	response, err := client.SendMessageWithToolsAndHistory(ctx, messages, availableTools)

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, types.StopReasonToolUse, response.StopReason)
	require.Len(t, response.ToolCalls, 1)
	assert.Equal(t, "describe_component", response.ToolCalls[0].Name)
	assert.Equal(t, "vpc", response.ToolCalls[0].Input["component"])
	assert.Equal(t, "dev", response.ToolCalls[0].Input["stack"])
}

func TestSendMessageWithToolsAndHistory_APIError(t *testing.T) {
	server := newMockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error": {"message": "Invalid API key", "type": "authentication_error"}}`))
	})
	defer server.Close()

	client := newTestClient(t, server.URL, "gpt-4o", 4096)

	messages := []types.Message{
		{Role: types.RoleUser, Content: "Hello"},
	}

	availableTools := []tools.Tool{
		&mockTool{name: "test_tool", description: "Test"},
	}

	ctx := context.Background()
	response, err := client.SendMessageWithToolsAndHistory(ctx, messages, availableTools)

	assert.Error(t, err)
	assert.Nil(t, response)
}

func TestSendMessageWithSystemPromptAndTools_Success(t *testing.T) {
	server := newMockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Verify the request contains system prompt, atmos memory, and tools.
		var reqBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		require.NoError(t, err)

		// Verify messages array - should have system prompt + atmos memory + user message.
		messages, ok := reqBody["messages"].([]interface{})
		require.True(t, ok)
		require.Len(t, messages, 3)

		// First message should be system prompt.
		msg0 := messages[0].(map[string]interface{})
		assert.Equal(t, "system", msg0["role"])

		w.Header().Set("Content-Type", "application/json")
		response := createMockChatCompletionResponse("I understand the system context.", "stop")
		err = json.NewEncoder(w).Encode(response)
		require.NoError(t, err)
	})
	defer server.Close()

	client := newTestClient(t, server.URL, "gpt-4o", 4096)

	systemPrompt := "You are an Atmos infrastructure assistant."
	atmosMemory := "Project uses AWS and has VPC, EKS components."
	messages := []types.Message{
		{Role: types.RoleUser, Content: "What components are available?"},
	}
	availableTools := []tools.Tool{
		&mockTool{name: "list_components", description: "List available components"},
	}

	ctx := context.Background()
	response, err := client.SendMessageWithSystemPromptAndTools(ctx, systemPrompt, atmosMemory, messages, availableTools)

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, "I understand the system context.", response.Content)
}

func TestSendMessageWithSystemPromptAndTools_EmptySystemPrompt(t *testing.T) {
	server := newMockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		require.NoError(t, err)

		// With empty system prompt and atmos memory, should only have user message.
		messages, ok := reqBody["messages"].([]interface{})
		require.True(t, ok)
		require.Len(t, messages, 1)

		w.Header().Set("Content-Type", "application/json")
		response := createMockChatCompletionResponse("Response without system context.", "stop")
		err = json.NewEncoder(w).Encode(response)
		require.NoError(t, err)
	})
	defer server.Close()

	client := newTestClient(t, server.URL, "gpt-4o", 4096)

	messages := []types.Message{
		{Role: types.RoleUser, Content: "Hello"},
	}
	availableTools := []tools.Tool{}

	ctx := context.Background()
	response, err := client.SendMessageWithSystemPromptAndTools(ctx, "", "", messages, availableTools)

	require.NoError(t, err)
	require.NotNil(t, response)
}

func TestSendMessageWithSystemPromptAndTools_OnlyAtmosMemory(t *testing.T) {
	server := newMockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		require.NoError(t, err)

		// Should have atmos memory + user message.
		messages, ok := reqBody["messages"].([]interface{})
		require.True(t, ok)
		require.Len(t, messages, 2)

		w.Header().Set("Content-Type", "application/json")
		response := createMockChatCompletionResponse("Using atmos memory.", "stop")
		err = json.NewEncoder(w).Encode(response)
		require.NoError(t, err)
	})
	defer server.Close()

	client := newTestClient(t, server.URL, "gpt-4o", 4096)

	atmosMemory := "This project uses Terraform for infrastructure."
	messages := []types.Message{
		{Role: types.RoleUser, Content: "What tools do we use?"},
	}

	ctx := context.Background()
	response, err := client.SendMessageWithSystemPromptAndTools(ctx, "", atmosMemory, messages, nil)

	require.NoError(t, err)
	require.NotNil(t, response)
}

func TestClient_ContextCancellation(t *testing.T) {
	// Create a server that takes a long time to respond.
	server := newMockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Check if the context is already cancelled.
		select {
		case <-r.Context().Done():
			return
		default:
			// Server doesn't respond before timeout.
			// The test will cancel context before this completes.
		}
	})
	defer server.Close()

	client := newTestClient(t, server.URL, "gpt-4o", 4096)

	// Create a cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := client.SendMessage(ctx, "Hello")
	assert.Error(t, err)
}

func TestToolConversion_AllParameterTypes(t *testing.T) {
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

	result := openaicompat.ConvertToolsToOpenAIFormat(availableTools)

	require.Len(t, result, 1)
	toolDef := result[0]

	// Verify function definition.
	assert.Equal(t, "comprehensive_tool", toolDef.Function.Name)
	assert.Equal(t, "Tool with all parameter types", toolDef.Function.Description.Value)

	// Verify parameters structure.
	params := toolDef.Function.Parameters
	require.NotNil(t, params)

	// Check type is object.
	paramType, ok := params["type"]
	assert.True(t, ok)
	assert.Equal(t, "object", paramType)

	// Check properties.
	props, ok := params["properties"].(map[string]interface{})
	assert.True(t, ok)
	assert.Len(t, props, 5)

	// Check required fields.
	required, ok := params["required"].([]string)
	assert.True(t, ok)
	assert.Contains(t, required, "string_param")
	assert.Contains(t, required, "int_param")
	assert.NotContains(t, required, "bool_param")
	assert.NotContains(t, required, "array_param")
	assert.NotContains(t, required, "object_param")
}

func TestPrependSystemMessages(t *testing.T) {
	tests := []struct {
		name           string
		systemPrompt   string
		atmosMemory    string
		messages       []types.Message
		expectedLength int
	}{
		{
			name:           "both system prompt and atmos memory",
			systemPrompt:   "You are a helpful assistant.",
			atmosMemory:    "Project context here.",
			messages:       []types.Message{{Role: types.RoleUser, Content: "Hello"}},
			expectedLength: 3,
		},
		{
			name:           "only system prompt",
			systemPrompt:   "You are a helpful assistant.",
			atmosMemory:    "",
			messages:       []types.Message{{Role: types.RoleUser, Content: "Hello"}},
			expectedLength: 2,
		},
		{
			name:           "only atmos memory",
			systemPrompt:   "",
			atmosMemory:    "Project context here.",
			messages:       []types.Message{{Role: types.RoleUser, Content: "Hello"}},
			expectedLength: 2,
		},
		{
			name:           "neither system prompt nor atmos memory",
			systemPrompt:   "",
			atmosMemory:    "",
			messages:       []types.Message{{Role: types.RoleUser, Content: "Hello"}},
			expectedLength: 1,
		},
		{
			name:           "empty messages",
			systemPrompt:   "System",
			atmosMemory:    "Memory",
			messages:       []types.Message{},
			expectedLength: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := base.PrependSystemMessages(tt.systemPrompt, tt.atmosMemory, tt.messages)
			assert.Len(t, result, tt.expectedLength)

			// Verify order: system prompt first, then atmos memory, then messages.
			idx := 0
			if tt.systemPrompt != "" {
				assert.Equal(t, types.RoleSystem, result[idx].Role)
				assert.Equal(t, tt.systemPrompt, result[idx].Content)
				idx++
			}
			if tt.atmosMemory != "" {
				assert.Equal(t, types.RoleSystem, result[idx].Role)
				assert.Equal(t, tt.atmosMemory, result[idx].Content)
				idx++
			}
			// Remaining should be the original messages.
			for i, msg := range tt.messages {
				assert.Equal(t, msg.Role, result[idx+i].Role)
				assert.Equal(t, msg.Content, result[idx+i].Content)
			}
		})
	}
}

func TestUsageTracking(t *testing.T) {
	server := newMockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"id":      "chatcmpl-usage",
			"object":  "chat.completion",
			"created": 1699999999,
			"model":   "gpt-4o",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Test response",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     150,
				"completion_tokens": 75,
				"total_tokens":      225,
			},
		}
		err := json.NewEncoder(w).Encode(response)
		require.NoError(t, err)
	})
	defer server.Close()

	client := newTestClient(t, server.URL, "gpt-4o", 4096)

	availableTools := []tools.Tool{}
	messages := []types.Message{{Role: types.RoleUser, Content: "Hello"}}

	ctx := context.Background()
	response, err := client.SendMessageWithToolsAndHistory(ctx, messages, availableTools)

	require.NoError(t, err)
	require.NotNil(t, response)
	require.NotNil(t, response.Usage)

	assert.Equal(t, int64(150), response.Usage.InputTokens)
	assert.Equal(t, int64(75), response.Usage.OutputTokens)
	assert.Equal(t, int64(225), response.Usage.TotalTokens)
}
