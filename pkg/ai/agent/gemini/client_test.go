package gemini

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

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
				Model:     "gemini-2.0-flash-exp",
				APIKeyEnv: "GEMINI_API_KEY",
				MaxTokens: 8192,
			},
		},
		{
			name: "Enabled configuration",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"gemini": {
								Model:     "gemini-1.5-pro",
								ApiKeyEnv: "CUSTOM_GEMINI_KEY",
								MaxTokens: 16384,
							},
						},
					},
				},
			},
			expectedConfig: &base.Config{
				Enabled:   true,
				Model:     "gemini-1.5-pro",
				APIKeyEnv: "CUSTOM_GEMINI_KEY",
				MaxTokens: 16384,
			},
		},
		{
			name: "Partial configuration",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"gemini": {
								Model: "gemini-1.5-flash",
							},
						},
					},
				},
			},
			expectedConfig: &base.Config{
				Enabled:   true,
				Model:     "gemini-1.5-flash",
				APIKeyEnv: "GEMINI_API_KEY",
				MaxTokens: 8192,
			},
		},
		{
			name: "Custom API key only",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"gemini": {
								ApiKeyEnv: "MY_GEMINI_API_KEY",
							},
						},
					},
				},
			},
			expectedConfig: &base.Config{
				Enabled:   true,
				Model:     "gemini-2.0-flash-exp",
				APIKeyEnv: "MY_GEMINI_API_KEY",
				MaxTokens: 8192,
			},
		},
		{
			name: "Max tokens only",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"gemini": {
								MaxTokens: 32768,
							},
						},
					},
				},
			},
			expectedConfig: &base.Config{
				Enabled:   true,
				Model:     "gemini-2.0-flash-exp",
				APIKeyEnv: "GEMINI_API_KEY",
				MaxTokens: 32768,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := base.ExtractConfig(tt.atmosConfig, ProviderName, base.ProviderDefaults{
				Model:     DefaultModel,
				APIKeyEnv: DefaultAPIKeyEnv,
				MaxTokens: DefaultMaxTokens,
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

func TestNewClient_MissingAPIKey(t *testing.T) {
	// Use a unique env var name that definitely does not exist.
	envVar := "NONEXISTENT_GEMINI_KEY_XYZZY_TEST"

	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"gemini": {
						ApiKeyEnv: envVar,
					},
				},
			},
		},
	}

	client, err := NewClient(context.TODO(), atmosConfig)
	assert.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "API key not found")
}

func TestClientGetters(t *testing.T) {
	config := &base.Config{
		Enabled:   true,
		Model:     "gemini-2.0-flash-exp",
		APIKeyEnv: "GEMINI_API_KEY",
		MaxTokens: 8192,
	}

	client := &Client{
		client: nil, // We don't need a real client for testing getters.
		config: config,
	}

	assert.Equal(t, "gemini-2.0-flash-exp", client.GetModel())
	assert.Equal(t, 8192, client.GetMaxTokens())
}

func TestDefaultConstants(t *testing.T) {
	assert.Equal(t, "gemini", ProviderName)
	assert.Equal(t, 8192, DefaultMaxTokens)
	assert.Equal(t, "gemini-2.0-flash-exp", DefaultModel)
	assert.Equal(t, "GEMINI_API_KEY", DefaultAPIKeyEnv)
}

func TestConfig_AllFields(t *testing.T) {
	config := &base.Config{
		Enabled:   true,
		Model:     "test-model",
		APIKeyEnv: "TEST_KEY",
		MaxTokens: 1000,
		BaseURL:   "https://api.example.com",
	}

	assert.True(t, config.Enabled)
	assert.Equal(t, "test-model", config.Model)
	assert.Equal(t, "TEST_KEY", config.APIKeyEnv)
	assert.Equal(t, 1000, config.MaxTokens)
	assert.Equal(t, "https://api.example.com", config.BaseURL)
}

func TestConvertMessagesToGeminiFormat_Empty(t *testing.T) {
	messages := []types.Message{}
	result := convertMessagesToGeminiFormat(messages)

	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestConvertMessagesToGeminiFormat_SingleUserMessage(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleUser, Content: "Hello, world!"},
	}

	result := convertMessagesToGeminiFormat(messages)

	require.Len(t, result, 1)
	assert.Equal(t, genai.RoleUser, result[0].Role)
	require.Len(t, result[0].Parts, 1)
	assert.Equal(t, "Hello, world!", result[0].Parts[0].Text)
}

func TestConvertMessagesToGeminiFormat_SingleAssistantMessage(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleAssistant, Content: "Hello! How can I help you?"},
	}

	result := convertMessagesToGeminiFormat(messages)

	require.Len(t, result, 1)
	assert.Equal(t, genai.RoleModel, result[0].Role)
	require.Len(t, result[0].Parts, 1)
	assert.Equal(t, "Hello! How can I help you?", result[0].Parts[0].Text)
}

func TestConvertMessagesToGeminiFormat_SystemMessageTreatedAsUser(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleSystem, Content: "You are a helpful assistant."},
	}

	result := convertMessagesToGeminiFormat(messages)

	// Gemini doesn't support system messages, so they're treated as user messages.
	require.Len(t, result, 1)
	assert.Equal(t, genai.RoleUser, result[0].Role)
	require.Len(t, result[0].Parts, 1)
	assert.Equal(t, "You are a helpful assistant.", result[0].Parts[0].Text)
}

func TestConvertMessagesToGeminiFormat_MultipleMessages(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleUser, Content: "What is 2+2?"},
		{Role: types.RoleAssistant, Content: "2+2 equals 4."},
		{Role: types.RoleUser, Content: "Thanks!"},
	}

	result := convertMessagesToGeminiFormat(messages)

	require.Len(t, result, 3)
	assert.Equal(t, genai.RoleUser, result[0].Role)
	assert.Equal(t, genai.RoleModel, result[1].Role)
	assert.Equal(t, genai.RoleUser, result[2].Role)
}

func TestConvertMessagesToGeminiFormat_UnknownRole(t *testing.T) {
	messages := []types.Message{
		{Role: "unknown", Content: "Unknown role message"},
	}

	result := convertMessagesToGeminiFormat(messages)

	// Unknown roles default to user.
	require.Len(t, result, 1)
	assert.Equal(t, genai.RoleUser, result[0].Role)
}

func TestConvertToolsToGeminiFormat_Empty(t *testing.T) {
	availableTools := []tools.Tool{}
	result := convertToolsToGeminiFormat(availableTools)

	assert.Nil(t, result)
}

func TestConvertToolsToGeminiFormat_SingleTool(t *testing.T) {
	availableTools := []tools.Tool{
		&mockTool{
			name:        "test_tool",
			description: "A test tool",
			parameters: []tools.Parameter{
				{Name: "param1", Type: tools.ParamTypeString, Description: "First param", Required: true},
			},
		},
	}

	result := convertToolsToGeminiFormat(availableTools)

	require.Len(t, result, 1)
	require.Len(t, result[0].FunctionDeclarations, 1)
	assert.Equal(t, "test_tool", result[0].FunctionDeclarations[0].Name)
	assert.Equal(t, "A test tool", result[0].FunctionDeclarations[0].Description)
}

func TestConvertToolsToGeminiFormat_MultipleTools(t *testing.T) {
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

	result := convertToolsToGeminiFormat(availableTools)

	// Gemini expects all functions in a single Tool.
	require.Len(t, result, 1)
	require.Len(t, result[0].FunctionDeclarations, 2)
	assert.Equal(t, "tool_a", result[0].FunctionDeclarations[0].Name)
	assert.Equal(t, "tool_b", result[0].FunctionDeclarations[1].Name)
}

func TestConvertToolsToGeminiFormat_AllParameterTypes(t *testing.T) {
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

	result := convertToolsToGeminiFormat(availableTools)

	require.Len(t, result, 1)
	require.Len(t, result[0].FunctionDeclarations, 1)
	funcDecl := result[0].FunctionDeclarations[0]

	assert.Equal(t, "comprehensive_tool", funcDecl.Name)
	assert.NotNil(t, funcDecl.Parameters)
	assert.Equal(t, genai.TypeObject, funcDecl.Parameters.Type)

	// Check properties.
	require.NotNil(t, funcDecl.Parameters.Properties)
	assert.Contains(t, funcDecl.Parameters.Properties, "string_param")
	assert.Contains(t, funcDecl.Parameters.Properties, "int_param")
	assert.Contains(t, funcDecl.Parameters.Properties, "bool_param")
	assert.Contains(t, funcDecl.Parameters.Properties, "array_param")
	assert.Contains(t, funcDecl.Parameters.Properties, "object_param")

	// Check parameter types.
	assert.Equal(t, genai.TypeString, funcDecl.Parameters.Properties["string_param"].Type)
	assert.Equal(t, genai.TypeInteger, funcDecl.Parameters.Properties["int_param"].Type)
	assert.Equal(t, genai.TypeBoolean, funcDecl.Parameters.Properties["bool_param"].Type)
	assert.Equal(t, genai.TypeArray, funcDecl.Parameters.Properties["array_param"].Type)
	assert.Equal(t, genai.TypeObject, funcDecl.Parameters.Properties["object_param"].Type)

	// Check required fields.
	require.Len(t, funcDecl.Parameters.Required, 2)
	assert.Contains(t, funcDecl.Parameters.Required, "string_param")
	assert.Contains(t, funcDecl.Parameters.Required, "int_param")
}

func TestConvertToolsToGeminiFormat_UnknownParameterType(t *testing.T) {
	availableTools := []tools.Tool{
		&mockTool{
			name:        "test_tool",
			description: "A test tool",
			parameters: []tools.Parameter{
				{Name: "unknown_param", Type: "custom", Description: "Unknown type param", Required: false},
			},
		},
	}

	result := convertToolsToGeminiFormat(availableTools)

	require.Len(t, result, 1)
	require.Len(t, result[0].FunctionDeclarations, 1)
	funcDecl := result[0].FunctionDeclarations[0]

	// Unknown types default to string.
	assert.Equal(t, genai.TypeString, funcDecl.Parameters.Properties["unknown_param"].Type)
}

func TestConvertToolsToGeminiFormat_NoParameters(t *testing.T) {
	availableTools := []tools.Tool{
		&mockTool{
			name:        "no_params_tool",
			description: "A tool with no parameters",
			parameters:  []tools.Parameter{},
		},
	}

	result := convertToolsToGeminiFormat(availableTools)

	require.Len(t, result, 1)
	require.Len(t, result[0].FunctionDeclarations, 1)
	funcDecl := result[0].FunctionDeclarations[0]

	assert.Equal(t, "no_params_tool", funcDecl.Name)
	assert.NotNil(t, funcDecl.Parameters)
	assert.Empty(t, funcDecl.Parameters.Properties)
	assert.Empty(t, funcDecl.Parameters.Required)
}

func TestParseGeminiResponse_TextOnly(t *testing.T) {
	response := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{Text: "Hello! How can I help you?"},
					},
				},
				FinishReason: genai.FinishReasonStop,
			},
		},
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     10,
			CandidatesTokenCount: 8,
			TotalTokenCount:      18,
		},
	}

	result, err := parseGeminiResponse(response)

	require.NoError(t, err)
	assert.Equal(t, "Hello! How can I help you?", result.Content)
	assert.Equal(t, types.StopReasonEndTurn, result.StopReason)
	assert.Empty(t, result.ToolCalls)
	require.NotNil(t, result.Usage)
	assert.Equal(t, int64(10), result.Usage.InputTokens)
	assert.Equal(t, int64(8), result.Usage.OutputTokens)
	assert.Equal(t, int64(18), result.Usage.TotalTokens)
}

func TestParseGeminiResponse_NoUsage(t *testing.T) {
	response := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{Text: "Response"},
					},
				},
				FinishReason: genai.FinishReasonStop,
			},
		},
		UsageMetadata: nil,
	}

	result, err := parseGeminiResponse(response)

	require.NoError(t, err)
	assert.Equal(t, "Response", result.Content)
	assert.Nil(t, result.Usage)
}

func TestParseGeminiResponse_NoCandidates(t *testing.T) {
	response := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{},
	}

	result, err := parseGeminiResponse(response)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no response candidates")
}

func TestParseGeminiResponse_MaxTokensFinishReason(t *testing.T) {
	response := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{Text: "Truncated..."},
					},
				},
				FinishReason: genai.FinishReasonMaxTokens,
			},
		},
	}

	result, err := parseGeminiResponse(response)

	require.NoError(t, err)
	assert.Equal(t, types.StopReasonMaxTokens, result.StopReason)
}

func TestParseGeminiResponse_SafetyFinishReason(t *testing.T) {
	response := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{Text: "Blocked content"},
					},
				},
				FinishReason: genai.FinishReasonSafety,
			},
		},
	}

	result, err := parseGeminiResponse(response)

	require.NoError(t, err)
	// Safety finish reason defaults to EndTurn.
	assert.Equal(t, types.StopReasonEndTurn, result.StopReason)
}

func TestParseGeminiResponse_RecitationFinishReason(t *testing.T) {
	response := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{Text: "Recitation content"},
					},
				},
				FinishReason: genai.FinishReasonRecitation,
			},
		},
	}

	result, err := parseGeminiResponse(response)

	require.NoError(t, err)
	// Recitation finish reason defaults to EndTurn.
	assert.Equal(t, types.StopReasonEndTurn, result.StopReason)
}

func TestParseGeminiResponse_OtherFinishReason(t *testing.T) {
	response := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{Text: "Other content"},
					},
				},
				FinishReason: genai.FinishReasonOther,
			},
		},
	}

	result, err := parseGeminiResponse(response)

	require.NoError(t, err)
	// Other finish reason defaults to EndTurn.
	assert.Equal(t, types.StopReasonEndTurn, result.StopReason)
}

func TestParseGeminiResponse_NilContent(t *testing.T) {
	response := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content:      nil,
				FinishReason: genai.FinishReasonStop,
			},
		},
	}

	result, err := parseGeminiResponse(response)

	require.NoError(t, err)
	assert.Empty(t, result.Content)
}

func TestParseGeminiResponse_EmptyParts(t *testing.T) {
	response := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []*genai.Part{},
				},
				FinishReason: genai.FinishReasonStop,
			},
		},
	}

	result, err := parseGeminiResponse(response)

	require.NoError(t, err)
	assert.Empty(t, result.Content)
}

func TestParseGeminiResponse_MultipleTextParts(t *testing.T) {
	response := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{Text: "First part. "},
						{Text: "Second part. "},
						{Text: "Third part."},
					},
				},
				FinishReason: genai.FinishReasonStop,
			},
		},
	}

	result, err := parseGeminiResponse(response)

	require.NoError(t, err)
	assert.Equal(t, "First part. Second part. Third part.", result.Content)
}

func TestParseGeminiResponse_WithCacheUsage(t *testing.T) {
	response := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{Text: "Cached response"},
					},
				},
				FinishReason: genai.FinishReasonStop,
			},
		},
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:        10,
			CandidatesTokenCount:    5,
			TotalTokenCount:         15,
			CachedContentTokenCount: 100,
		},
	}

	result, err := parseGeminiResponse(response)

	require.NoError(t, err)
	require.NotNil(t, result.Usage)
	assert.Equal(t, int64(10), result.Usage.InputTokens)
	assert.Equal(t, int64(5), result.Usage.OutputTokens)
	assert.Equal(t, int64(15), result.Usage.TotalTokens)
	assert.Equal(t, int64(100), result.Usage.CacheReadTokens)
	assert.Equal(t, int64(0), result.Usage.CacheCreationTokens) // Gemini doesn't report this separately.
}

func TestGeminiModels(t *testing.T) {
	// Test various Gemini model names.
	models := []struct {
		modelID     string
		description string
	}{
		{"gemini-2.0-flash-exp", "Gemini 2.0 Flash Experimental"},
		{"gemini-1.5-pro", "Gemini 1.5 Pro"},
		{"gemini-1.5-flash", "Gemini 1.5 Flash"},
		{"gemini-1.0-pro", "Gemini 1.0 Pro"},
	}

	for _, m := range models {
		t.Run(m.description, func(t *testing.T) {
			config := &base.Config{
				Enabled:   true,
				Model:     m.modelID,
				APIKeyEnv: DefaultAPIKeyEnv,
				MaxTokens: DefaultMaxTokens,
			}

			client := &Client{
				client: nil,
				config: config,
			}

			assert.Equal(t, m.modelID, client.GetModel())
		})
	}
}

func TestParameterTypeMapping(t *testing.T) {
	// Test that all parameter types are correctly mapped to Gemini types.
	typeTests := []struct {
		inputType    tools.ParamType
		expectedType genai.Type
	}{
		{tools.ParamTypeString, genai.TypeString},
		{tools.ParamTypeInt, genai.TypeInteger},
		{tools.ParamTypeBool, genai.TypeBoolean},
		{tools.ParamTypeArray, genai.TypeArray},
		{tools.ParamTypeObject, genai.TypeObject},
	}

	for _, tt := range typeTests {
		t.Run(string(tt.inputType), func(t *testing.T) {
			availableTools := []tools.Tool{
				&mockTool{
					name:        "test_tool",
					description: "Test tool",
					parameters: []tools.Parameter{
						{Name: "test_param", Type: tt.inputType, Description: "Test param", Required: true},
					},
				},
			}

			result := convertToolsToGeminiFormat(availableTools)

			require.Len(t, result, 1)
			require.Len(t, result[0].FunctionDeclarations, 1)
			assert.Equal(t, tt.expectedType, result[0].FunctionDeclarations[0].Parameters.Properties["test_param"].Type)
		})
	}
}

func TestRoleMapping(t *testing.T) {
	// Test that all message roles are correctly mapped to Gemini roles.
	roleTests := []struct {
		inputRole    string
		expectedRole string
	}{
		{types.RoleUser, genai.RoleUser},
		{types.RoleAssistant, genai.RoleModel},
		{types.RoleSystem, genai.RoleUser}, // System messages treated as user.
		{"unknown", genai.RoleUser},        // Unknown defaults to user.
	}

	for _, tt := range roleTests {
		t.Run(tt.inputRole, func(t *testing.T) {
			messages := []types.Message{
				{Role: tt.inputRole, Content: "Test message"},
			}

			result := convertMessagesToGeminiFormat(messages)

			require.Len(t, result, 1)
			assert.Equal(t, tt.expectedRole, result[0].Role)
		})
	}
}

func TestConvertToolsToGeminiFormat_NumberType(t *testing.T) {
	// Test that 'number' type is mapped correctly.
	availableTools := []tools.Tool{
		&mockTool{
			name:        "test_tool",
			description: "Test tool",
			parameters: []tools.Parameter{
				{Name: "float_param", Type: "number", Description: "Floating point param", Required: true},
			},
		},
	}

	result := convertToolsToGeminiFormat(availableTools)

	require.Len(t, result, 1)
	require.Len(t, result[0].FunctionDeclarations, 1)
	assert.Equal(t, genai.TypeNumber, result[0].FunctionDeclarations[0].Parameters.Properties["float_param"].Type)
}

func TestGeminiMaxTokensVariations(t *testing.T) {
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
			}

			client := &Client{
				client: nil,
				config: config,
			}

			assert.Equal(t, tt.expected, client.GetMaxTokens())
		})
	}
}
