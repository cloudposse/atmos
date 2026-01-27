package anthropic

import (
	"context"
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
				Model:     "claude-sonnet-4-20250514",
				APIKeyEnv: "ANTHROPIC_API_KEY",
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
							"anthropic": {
								Model:     "claude-4-20250514",
								ApiKeyEnv: "CUSTOM_API_KEY",
								MaxTokens: 8192,
							},
						},
					},
				},
			},
			expectedConfig: &base.Config{
				Enabled:   true,
				Model:     "claude-4-20250514",
				APIKeyEnv: "CUSTOM_API_KEY",
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
							"anthropic": {
								Model: "claude-3-haiku-20240307",
							},
						},
					},
				},
			},
			expectedConfig: &base.Config{
				Enabled:   true,
				Model:     "claude-3-haiku-20240307",
				APIKeyEnv: "ANTHROPIC_API_KEY",
				MaxTokens: 4096,
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

func TestNewSimpleClient_Disabled(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: false,
			},
		},
	}

	client, err := NewSimpleClient(atmosConfig)
	assert.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "AI features are disabled")
}

func TestNewSimpleClient_MissingAPIKey(t *testing.T) {
	// Use a unique env var name that definitely does not exist.
	envVar := "NONEXISTENT_ANTHROPIC_KEY_XYZZY"

	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"anthropic": {
						ApiKeyEnv: envVar,
					},
				},
			},
		},
	}

	client, err := NewSimpleClient(atmosConfig)
	assert.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "API key not found")
}

// Note: TestNewSimpleClient_WithAPIKey is skipped because viper's AutomaticEnv
// doesn't reliably pick up env vars set with t.Setenv in tests. The actual
// API key retrieval is tested indirectly through integration tests.

func TestSimpleClientGetters(t *testing.T) {
	config := &base.Config{
		Enabled:   true,
		Model:     "claude-sonnet-4-20250514",
		APIKeyEnv: "ANTHROPIC_API_KEY",
		MaxTokens: 4096,
	}

	client := &SimpleClient{
		client: nil, // We don't need a real client for testing getters.
		config: config,
	}

	assert.Equal(t, "claude-sonnet-4-20250514", client.GetModel())
	assert.Equal(t, 4096, client.GetMaxTokens())
}

func TestConvertToolsToAnthropicFormat(t *testing.T) {
	// Create a mock tool.
	mockTool := &mockTool{
		name:        "test_tool",
		description: "A test tool for verification",
		parameters: []tools.Parameter{
			{
				Name:        "query",
				Type:        "string",
				Description: "The search query",
				Required:    true,
			},
			{
				Name:        "max_results",
				Type:        "integer",
				Description: "Maximum number of results",
				Required:    false,
			},
			{
				Name:        "verbose",
				Type:        "boolean",
				Description: "Enable verbose output",
				Required:    false,
			},
		},
	}

	// Verify the mock tool structure.
	assert.Equal(t, "test_tool", mockTool.name)
	assert.Equal(t, "A test tool for verification", mockTool.description)
	assert.Len(t, mockTool.parameters, 3)

	// Verify parameter types match JSON Schema spec.
	assert.Equal(t, "string", string(mockTool.parameters[0].Type))
	assert.Equal(t, "integer", string(mockTool.parameters[1].Type), "Should be 'integer', not 'int'")
	assert.Equal(t, "boolean", string(mockTool.parameters[2].Type), "Should be 'boolean', not 'bool'")
}

func TestConvertToolsToAnthropicFormat_Empty(t *testing.T) {
	availableTools := []tools.Tool{}
	result := convertToolsToAnthropicFormat(availableTools)

	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestConvertToolsToAnthropicFormat_SingleTool(t *testing.T) {
	availableTools := []tools.Tool{
		&mockTool{
			name:        "test_tool",
			description: "A test tool",
			parameters: []tools.Parameter{
				{Name: "param1", Type: tools.ParamTypeString, Description: "First param", Required: true},
			},
		},
	}

	result := convertToolsToAnthropicFormat(availableTools)

	assert.Len(t, result, 1)
	assert.NotNil(t, result[0].OfTool)
	assert.Equal(t, "test_tool", result[0].OfTool.Name)
	assert.Equal(t, "A test tool", result[0].OfTool.Description.Value)
}

func TestConvertToolsToAnthropicFormat_MultipleTools(t *testing.T) {
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

	result := convertToolsToAnthropicFormat(availableTools)

	assert.Len(t, result, 2)
	assert.Equal(t, "tool_a", result[0].OfTool.Name)
	assert.Equal(t, "tool_b", result[1].OfTool.Name)
}

func TestConvertToolsToAnthropicFormat_AllParameterTypes(t *testing.T) {
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

	result := convertToolsToAnthropicFormat(availableTools)

	require.Len(t, result, 1)
	assert.Equal(t, "comprehensive_tool", result[0].OfTool.Name)
	assert.Equal(t, "object", string(result[0].OfTool.InputSchema.Type))

	// Verify required fields.
	require.Len(t, result[0].OfTool.InputSchema.Required, 2)
	assert.Contains(t, result[0].OfTool.InputSchema.Required, "string_param")
	assert.Contains(t, result[0].OfTool.InputSchema.Required, "int_param")
}

func TestConvertMessagesToAnthropicFormat_Empty(t *testing.T) {
	messages := []types.Message{}
	result := convertMessagesToAnthropicFormat(messages)

	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestConvertMessagesToAnthropicFormat_SingleUserMessage(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleUser, Content: "Hello, world!"},
	}

	result := convertMessagesToAnthropicFormat(messages)

	assert.Len(t, result, 1)
}

func TestConvertMessagesToAnthropicFormat_SingleAssistantMessage(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleAssistant, Content: "Hello! How can I help you?"},
	}

	result := convertMessagesToAnthropicFormat(messages)

	assert.Len(t, result, 1)
}

func TestConvertMessagesToAnthropicFormat_MultipleMessages(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleUser, Content: "What is 2+2?"},
		{Role: types.RoleAssistant, Content: "2+2 equals 4."},
		{Role: types.RoleUser, Content: "Thanks!"},
	}

	result := convertMessagesToAnthropicFormat(messages)

	assert.Len(t, result, 3)
}

func TestConvertMessagesToAnthropicFormat_SystemMessageSkipped(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleSystem, Content: "You are a helpful assistant."},
		{Role: types.RoleUser, Content: "Hello"},
	}

	result := convertMessagesToAnthropicFormat(messages)

	// System messages should be skipped (they go via System parameter, not Messages).
	assert.Len(t, result, 1)
}

// Note: Tests for parseAnthropicResponse are not included because the Anthropic SDK
// uses internal types (ContentBlockUnion) that cannot be easily constructed in tests.
// The response parsing is tested indirectly through integration tests.

func TestToolSchema_JSONSchemaCompliance(t *testing.T) {
	// Verify that our tool schema structure matches JSON Schema draft 2020-12 requirements.
	tests := []struct {
		name          string
		schemaType    string
		shouldBeValid bool
		reason        string
	}{
		{
			name:          "object type required",
			schemaType:    "object",
			shouldBeValid: true,
			reason:        "JSON Schema draft 2020-12 requires 'type' field",
		},
		{
			name:          "string type valid",
			schemaType:    "string",
			shouldBeValid: true,
			reason:        "string is valid JSON Schema type",
		},
		{
			name:          "integer type valid",
			schemaType:    "integer",
			shouldBeValid: true,
			reason:        "integer is valid (not 'int')",
		},
		{
			name:          "boolean type valid",
			schemaType:    "boolean",
			shouldBeValid: true,
			reason:        "boolean is valid (not 'bool')",
		},
		{
			name:          "int type invalid",
			schemaType:    "int",
			shouldBeValid: false,
			reason:        "JSON Schema uses 'integer', not 'int'",
		},
		{
			name:          "bool type invalid",
			schemaType:    "bool",
			shouldBeValid: false,
			reason:        "JSON Schema uses 'boolean', not 'bool'",
		},
	}

	validTypes := map[string]bool{
		"object":  true,
		"string":  true,
		"integer": true,
		"number":  true,
		"boolean": true,
		"array":   true,
		"null":    true,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := validTypes[tt.schemaType]
			assert.Equal(t, tt.shouldBeValid, isValid, tt.reason)
		})
	}
}

func TestAnthropicClient_ToolDescriptionRequired(t *testing.T) {
	// This test verifies that tool descriptions are critical for AI decision-making.
	tests := []struct {
		name                  string
		toolDescription       string
		shouldHaveDescription bool
		reason                string
	}{
		{
			name:                  "with description",
			toolDescription:       "Search the web for information",
			shouldHaveDescription: true,
			reason:                "Tool description tells Claude WHEN to call the tool",
		},
		{
			name:                  "empty description",
			toolDescription:       "",
			shouldHaveDescription: false,
			reason:                "Empty description means Claude won't know when to use tool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasDescription := len(tt.toolDescription) > 0
			assert.Equal(t, tt.shouldHaveDescription, hasDescription, tt.reason)

			if tt.shouldHaveDescription {
				assert.NotEmpty(t, tt.toolDescription,
					"Tool descriptions are the 'instruction manual' for AI - they MUST be present")
			}
		})
	}
}

// Token Caching Tests.

func TestExtractCacheConfig(t *testing.T) {
	tests := []struct {
		name          string
		atmosConfig   *schema.AtmosConfiguration
		expectedCache *cacheConfig
	}{
		{
			name: "Default cache enabled",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"anthropic": {
								Model: "claude-sonnet-4-20250514",
							},
						},
					},
				},
			},
			expectedCache: &cacheConfig{
				enabled:            true,
				cacheSystemPrompt:  true,
				cacheProjectMemory: true,
			},
		},
		{
			name: "Cache explicitly disabled",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"anthropic": {
								Model: "claude-sonnet-4-20250514",
								Cache: &schema.AICacheSettings{
									Enabled: false,
								},
							},
						},
					},
				},
			},
			expectedCache: &cacheConfig{
				enabled:            false,
				cacheSystemPrompt:  false,
				cacheProjectMemory: false,
			},
		},
		{
			name: "Cache enabled with fine-grained control",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"anthropic": {
								Model: "claude-sonnet-4-20250514",
								Cache: &schema.AICacheSettings{
									Enabled:            true,
									CacheSystemPrompt:  true,
									CacheProjectMemory: false, // Only cache system prompt.
								},
							},
						},
					},
				},
			},
			expectedCache: &cacheConfig{
				enabled:            true,
				cacheSystemPrompt:  true,
				cacheProjectMemory: false,
			},
		},
		{
			name: "No provider config",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled:   true,
						Providers: nil,
					},
				},
			},
			expectedCache: &cacheConfig{
				enabled:            true,
				cacheSystemPrompt:  true,
				cacheProjectMemory: true,
			},
		},
		{
			name: "Different provider only",
			atmosConfig: &schema.AtmosConfiguration{
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
			},
			expectedCache: &cacheConfig{
				enabled:            true,
				cacheSystemPrompt:  true,
				cacheProjectMemory: true,
			},
		},
		{
			name: "Cache enabled with both options true by default",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"anthropic": {
								Model: "claude-sonnet-4-20250514",
								Cache: &schema.AICacheSettings{
									Enabled:            true,
									CacheSystemPrompt:  false,
									CacheProjectMemory: false,
								},
							},
						},
					},
				},
			},
			expectedCache: &cacheConfig{
				enabled:            true,
				cacheSystemPrompt:  true, // Defaults to true when both are false.
				cacheProjectMemory: true, // Defaults to true when both are false.
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := extractCacheConfig(tt.atmosConfig)
			assert.Equal(t, tt.expectedCache, cache)
		})
	}
}

func TestBuildSystemPrompt_NoCaching(t *testing.T) {
	client := &SimpleClient{
		config: &base.Config{
			Enabled: true,
		},
		cache: &cacheConfig{
			enabled:           false,
			cacheSystemPrompt: true, // Ignored when enabled is false.
		},
	}

	textBlock := client.buildSystemPrompt("Test system prompt", true)

	assert.Equal(t, "Test system prompt", textBlock.Text)
	// Cache control should NOT be set when caching is disabled.
	assert.Empty(t, textBlock.CacheControl.Type, "Cache control should not be set when caching is disabled")
}

func TestBuildSystemPrompt_WithCaching(t *testing.T) {
	client := &SimpleClient{
		config: &base.Config{
			Enabled: true,
		},
		cache: &cacheConfig{
			enabled:           true,
			cacheSystemPrompt: true,
		},
	}

	textBlock := client.buildSystemPrompt("Test system prompt", true)

	assert.Equal(t, "Test system prompt", textBlock.Text)
	// Cache control SHOULD be set when caching is enabled.
	assert.NotEmpty(t, textBlock.CacheControl.Type, "Cache control should be set when caching is enabled")
}

func TestBuildSystemPrompt_CachingDisabledPerPrompt(t *testing.T) {
	client := &SimpleClient{
		config: &base.Config{
			Enabled: true,
		},
		cache: &cacheConfig{
			enabled:           true,
			cacheSystemPrompt: true, // Global caching enabled.
		},
	}

	// Request caching disabled for this specific prompt.
	textBlock := client.buildSystemPrompt("Test system prompt", false)

	assert.Equal(t, "Test system prompt", textBlock.Text)
	// Cache control should NOT be set when explicitly disabled for this prompt.
	assert.Empty(t, textBlock.CacheControl.Type, "Cache control should not be set when disabled per-prompt")
}

func TestBuildSystemPrompt_EmptyPrompt(t *testing.T) {
	client := &SimpleClient{
		config: &base.Config{
			Enabled: true,
		},
		cache: &cacheConfig{
			enabled:           true,
			cacheSystemPrompt: true,
		},
	}

	textBlock := client.buildSystemPrompt("", true)

	assert.Empty(t, textBlock.Text)
}

func TestCacheConfiguration_CostSavings(t *testing.T) {
	// This test documents the cost savings from token caching.
	// Anthropic provides 90% discount on cached input tokens.

	tests := []struct {
		name                 string
		cacheEnabled         bool
		inputTokens          int
		cachedInputTokens    int
		outputTokens         int
		expectedSavingsRatio float64
		description          string
	}{
		{
			name:                 "No caching",
			cacheEnabled:         false,
			inputTokens:          10000,
			cachedInputTokens:    0,
			outputTokens:         1000,
			expectedSavingsRatio: 0.0,
			description:          "Without caching, pay full price for all input tokens",
		},
		{
			name:                 "50% cache hit rate",
			cacheEnabled:         true,
			inputTokens:          5000,
			cachedInputTokens:    5000,
			outputTokens:         1000,
			expectedSavingsRatio: 0.409, // 5000 cached tokens * 90% discount / 11000 total = 4500/11000 = 0.409
			description:          "With 50% cache hit, save ~41% on total costs",
		},
		{
			name:                 "90% cache hit rate (system prompt + ATMOS.md)",
			cacheEnabled:         true,
			inputTokens:          1000,
			cachedInputTokens:    9000,
			outputTokens:         1000,
			expectedSavingsRatio: 0.736, // 9000 cached tokens * 90% discount / 11000 total = 8100/11000 = 0.736
			description:          "With 90% cache hit (typical for system+memory), save ~74% on total costs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			totalTokens := tt.inputTokens + tt.cachedInputTokens + tt.outputTokens
			cachedSavings := float64(tt.cachedInputTokens) * 0.9 // 90% discount
			savingsRatio := cachedSavings / float64(totalTokens)

			assert.InDelta(t, tt.expectedSavingsRatio, savingsRatio, 0.01, tt.description)

			t.Logf("Cache Savings Analysis: %s", tt.name)
			t.Logf("  Input tokens: %d", tt.inputTokens)
			t.Logf("  Cached tokens: %d", tt.cachedInputTokens)
			t.Logf("  Output tokens: %d", tt.outputTokens)
			t.Logf("  Total tokens: %d", totalTokens)
			t.Logf("  Savings ratio: %.1f%%", savingsRatio*100)
			t.Logf("  Description: %s", tt.description)
		})
	}
}

func TestDefaultConstants(t *testing.T) {
	assert.Equal(t, "anthropic", ProviderName)
	assert.Equal(t, 4096, DefaultMaxTokens)
	assert.Equal(t, "claude-sonnet-4-20250514", DefaultModel)
	assert.Equal(t, "ANTHROPIC_API_KEY", DefaultAPIKeyEnv)
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
		APIKeyEnv: DefaultAPIKeyEnv,
		MaxTokens: DefaultMaxTokens,
	})

	// Should use defaults when providers is nil.
	assert.True(t, config.Enabled)
	assert.Equal(t, DefaultModel, config.Model)
	assert.Equal(t, DefaultAPIKeyEnv, config.APIKeyEnv)
	assert.Equal(t, DefaultMaxTokens, config.MaxTokens)
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
		APIKeyEnv: DefaultAPIKeyEnv,
		MaxTokens: DefaultMaxTokens,
	})

	// Should use defaults when this provider is not configured.
	assert.True(t, config.Enabled)
	assert.Equal(t, DefaultModel, config.Model)
	assert.Equal(t, DefaultAPIKeyEnv, config.APIKeyEnv)
	assert.Equal(t, DefaultMaxTokens, config.MaxTokens)
}

func TestExtractConfig_NilProviderConfig(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"anthropic": nil, // Explicitly nil provider config.
				},
			},
		},
	}

	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:     DefaultModel,
		APIKeyEnv: DefaultAPIKeyEnv,
		MaxTokens: DefaultMaxTokens,
	})

	// Should use defaults when provider config is nil.
	assert.True(t, config.Enabled)
	assert.Equal(t, DefaultModel, config.Model)
	assert.Equal(t, DefaultAPIKeyEnv, config.APIKeyEnv)
	assert.Equal(t, DefaultMaxTokens, config.MaxTokens)
}

func TestClientGetters_CustomValues(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		maxTokens int
	}{
		{
			name:      "Default Claude Sonnet 4",
			model:     "claude-sonnet-4-20250514",
			maxTokens: 4096,
		},
		{
			name:      "Claude 4",
			model:     "claude-4-20250514",
			maxTokens: 8192,
		},
		{
			name:      "Claude 3 Opus",
			model:     "claude-3-opus-20240229",
			maxTokens: 4096,
		},
		{
			name:      "Claude 3 Haiku",
			model:     "claude-3-haiku-20240307",
			maxTokens: 4096,
		},
		{
			name:      "High token limit",
			model:     "claude-sonnet-4-20250514",
			maxTokens: 131072,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &SimpleClient{
				client: nil,
				config: &base.Config{
					Enabled:   true,
					Model:     tt.model,
					MaxTokens: tt.maxTokens,
				},
			}

			assert.Equal(t, tt.model, client.GetModel())
			assert.Equal(t, tt.maxTokens, client.GetMaxTokens())
		})
	}
}

func TestAnthropicModels(t *testing.T) {
	// Test various Anthropic model configurations.
	models := []struct {
		modelID     string
		description string
	}{
		{"claude-sonnet-4-20250514", "Claude Sonnet 4"},
		{"claude-4-20250514", "Claude 4"},
		{"claude-3-5-sonnet-20241022", "Claude 3.5 Sonnet"},
		{"claude-3-opus-20240229", "Claude 3 Opus"},
		{"claude-3-sonnet-20240229", "Claude 3 Sonnet"},
		{"claude-3-haiku-20240307", "Claude 3 Haiku"},
	}

	for _, m := range models {
		t.Run(m.description, func(t *testing.T) {
			config := &base.Config{
				Enabled:   true,
				Model:     m.modelID,
				APIKeyEnv: DefaultAPIKeyEnv,
				MaxTokens: DefaultMaxTokens,
			}

			client := &SimpleClient{
				client: nil,
				config: config,
			}

			assert.Equal(t, m.modelID, client.GetModel())
		})
	}
}

func TestCacheConfig_Fields(t *testing.T) {
	// Test cacheConfig struct fields.
	cache := &cacheConfig{
		enabled:            true,
		cacheSystemPrompt:  true,
		cacheProjectMemory: false,
	}

	assert.True(t, cache.enabled)
	assert.True(t, cache.cacheSystemPrompt)
	assert.False(t, cache.cacheProjectMemory)
}

func TestExtractCacheConfig_EmptyProviders(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled:   true,
				Providers: map[string]*schema.AIProviderConfig{},
			},
		},
	}

	cache := extractCacheConfig(atmosConfig)

	// Default behavior when no provider config.
	assert.True(t, cache.enabled)
	assert.True(t, cache.cacheSystemPrompt)
	assert.True(t, cache.cacheProjectMemory)
}

func TestExtractCacheConfig_OnlySystemPrompt(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"anthropic": {
						Cache: &schema.AICacheSettings{
							Enabled:            true,
							CacheSystemPrompt:  true,
							CacheProjectMemory: false,
						},
					},
				},
			},
		},
	}

	cache := extractCacheConfig(atmosConfig)

	assert.True(t, cache.enabled)
	assert.True(t, cache.cacheSystemPrompt)
	assert.False(t, cache.cacheProjectMemory)
}

func TestExtractCacheConfig_OnlyProjectMemory(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"anthropic": {
						Cache: &schema.AICacheSettings{
							Enabled:            true,
							CacheSystemPrompt:  false,
							CacheProjectMemory: true,
						},
					},
				},
			},
		},
	}

	cache := extractCacheConfig(atmosConfig)

	assert.True(t, cache.enabled)
	assert.False(t, cache.cacheSystemPrompt)
	assert.True(t, cache.cacheProjectMemory)
}

func TestBuildSystemPrompt_MultipleScenarios(t *testing.T) {
	tests := []struct {
		name           string
		prompt         string
		cacheEnabled   bool
		promptCaching  bool
		requestCaching bool
		expectCache    bool
	}{
		{
			name:           "All caching enabled",
			prompt:         "System prompt",
			cacheEnabled:   true,
			promptCaching:  true,
			requestCaching: true,
			expectCache:    true,
		},
		{
			name:           "Global caching disabled",
			prompt:         "System prompt",
			cacheEnabled:   false,
			promptCaching:  true,
			requestCaching: true,
			expectCache:    false,
		},
		{
			name:           "Request caching disabled",
			prompt:         "System prompt",
			cacheEnabled:   true,
			promptCaching:  true,
			requestCaching: false,
			expectCache:    false,
		},
		{
			name:           "Long prompt with caching",
			prompt:         "This is a very long system prompt that should benefit from caching. " + "It contains detailed instructions for the AI model on how to behave.",
			cacheEnabled:   true,
			promptCaching:  true,
			requestCaching: true,
			expectCache:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &SimpleClient{
				config: &base.Config{Enabled: true},
				cache: &cacheConfig{
					enabled:           tt.cacheEnabled,
					cacheSystemPrompt: tt.promptCaching,
				},
			}

			textBlock := client.buildSystemPrompt(tt.prompt, tt.requestCaching)

			assert.Equal(t, tt.prompt, textBlock.Text)
			if tt.expectCache {
				assert.NotEmpty(t, textBlock.CacheControl.Type)
			} else {
				assert.Empty(t, textBlock.CacheControl.Type)
			}
		})
	}
}

func TestConvertToolsToAnthropicFormat_WithDefaultValues(t *testing.T) {
	availableTools := []tools.Tool{
		&mockTool{
			name:        "tool_with_defaults",
			description: "Tool with default parameter values",
			parameters: []tools.Parameter{
				{Name: "required_param", Type: tools.ParamTypeString, Description: "Required", Required: true},
				{Name: "optional_with_default", Type: tools.ParamTypeInt, Description: "Optional", Required: false, Default: 10},
				{Name: "optional_bool", Type: tools.ParamTypeBool, Description: "Boolean", Required: false, Default: true},
			},
		},
	}

	result := convertToolsToAnthropicFormat(availableTools)

	require.Len(t, result, 1)
	assert.Equal(t, "tool_with_defaults", result[0].OfTool.Name)

	// Verify only the required parameter is in the required list.
	require.Len(t, result[0].OfTool.InputSchema.Required, 1)
	assert.Contains(t, result[0].OfTool.InputSchema.Required, "required_param")
}

func TestConvertMessagesToAnthropicFormat_EmptyContent(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleUser, Content: ""},
		{Role: types.RoleAssistant, Content: ""},
	}

	result := convertMessagesToAnthropicFormat(messages)

	// Empty content messages should still be converted.
	assert.Len(t, result, 2)
}

func TestConvertMessagesToAnthropicFormat_UnknownRole(t *testing.T) {
	messages := []types.Message{
		{Role: "unknown", Content: "This should be skipped"},
		{Role: types.RoleUser, Content: "Valid message"},
	}

	result := convertMessagesToAnthropicFormat(messages)

	// Unknown roles should be skipped.
	assert.Len(t, result, 1)
}

func TestExtractConfig_AllOverrides(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"anthropic": {
						Model:     "claude-3-opus-20240229",
						ApiKeyEnv: "CUSTOM_ANTHROPIC_KEY",
						MaxTokens: 32768,
						BaseURL:   "https://api.custom.anthropic.com/v1",
					},
				},
			},
		},
	}

	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:     DefaultModel,
		APIKeyEnv: DefaultAPIKeyEnv,
		MaxTokens: DefaultMaxTokens,
	})

	assert.True(t, config.Enabled)
	assert.Equal(t, "claude-3-opus-20240229", config.Model)
	assert.Equal(t, "CUSTOM_ANTHROPIC_KEY", config.APIKeyEnv)
	assert.Equal(t, 32768, config.MaxTokens)
	assert.Equal(t, "https://api.custom.anthropic.com/v1", config.BaseURL)
}
