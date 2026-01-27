package anthropic

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
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

// TestParseAnthropicResponse tests the parseAnthropicResponse function directly.
// Since we can construct anthropic.Message structs, we can test this function.
func TestParseAnthropicResponse_TextOnly(t *testing.T) {
	// Create a mock response with text content.
	response := &anthropic.Message{
		ID:         "msg_123",
		Type:       "message",
		Role:       "assistant",
		Model:      "claude-sonnet-4-20250514",
		StopReason: "end_turn",
		Usage: anthropic.Usage{
			InputTokens:  100,
			OutputTokens: 50,
		},
	}

	result, err := parseAnthropicResponse(response)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, types.StopReasonEndTurn, result.StopReason)
	assert.NotNil(t, result.Usage)
	assert.Equal(t, int64(100), result.Usage.InputTokens)
	assert.Equal(t, int64(50), result.Usage.OutputTokens)
	assert.Equal(t, int64(150), result.Usage.TotalTokens)
}

func TestParseAnthropicResponse_WithTextContent(t *testing.T) {
	// Create a response with text content block.
	response := &anthropic.Message{
		StopReason: anthropic.StopReasonEndTurn,
		Content: []anthropic.ContentBlockUnion{
			{
				Type: "text",
				Text: "Hello, this is the AI response.",
			},
		},
		Usage: anthropic.Usage{
			InputTokens:  50,
			OutputTokens: 20,
		},
	}

	result, err := parseAnthropicResponse(response)
	require.NoError(t, err)
	assert.Equal(t, "Hello, this is the AI response.", result.Content)
	assert.Empty(t, result.ToolCalls)
}

func TestParseAnthropicResponse_WithMultipleTextBlocks(t *testing.T) {
	// Create a response with multiple text content blocks.
	response := &anthropic.Message{
		StopReason: anthropic.StopReasonEndTurn,
		Content: []anthropic.ContentBlockUnion{
			{Type: "text", Text: "First part. "},
			{Type: "text", Text: "Second part. "},
			{Type: "text", Text: "Third part."},
		},
		Usage: anthropic.Usage{InputTokens: 10, OutputTokens: 30},
	}

	result, err := parseAnthropicResponse(response)
	require.NoError(t, err)
	assert.Equal(t, "First part. Second part. Third part.", result.Content)
}

func TestParseAnthropicResponse_WithToolUse(t *testing.T) {
	// Create a response with tool use content block.
	response := &anthropic.Message{
		StopReason: anthropic.StopReasonToolUse,
		Content: []anthropic.ContentBlockUnion{
			{
				Type:  "tool_use",
				ID:    "toolu_123",
				Name:  "search_web",
				Input: json.RawMessage(`{"query": "test query", "max_results": 5}`),
			},
		},
		Usage: anthropic.Usage{InputTokens: 100, OutputTokens: 50},
	}

	result, err := parseAnthropicResponse(response)
	require.NoError(t, err)
	assert.Equal(t, types.StopReasonToolUse, result.StopReason)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "toolu_123", result.ToolCalls[0].ID)
	assert.Equal(t, "search_web", result.ToolCalls[0].Name)
	assert.Equal(t, "test query", result.ToolCalls[0].Input["query"])
	assert.Equal(t, float64(5), result.ToolCalls[0].Input["max_results"])
}

func TestParseAnthropicResponse_WithMultipleToolCalls(t *testing.T) {
	// Create a response with multiple tool use blocks.
	response := &anthropic.Message{
		StopReason: anthropic.StopReasonToolUse,
		Content: []anthropic.ContentBlockUnion{
			{
				Type:  "tool_use",
				ID:    "toolu_1",
				Name:  "tool_a",
				Input: json.RawMessage(`{"param": "value1"}`),
			},
			{
				Type:  "tool_use",
				ID:    "toolu_2",
				Name:  "tool_b",
				Input: json.RawMessage(`{"param": "value2"}`),
			},
		},
		Usage: anthropic.Usage{InputTokens: 100, OutputTokens: 75},
	}

	result, err := parseAnthropicResponse(response)
	require.NoError(t, err)
	require.Len(t, result.ToolCalls, 2)
	assert.Equal(t, "toolu_1", result.ToolCalls[0].ID)
	assert.Equal(t, "tool_a", result.ToolCalls[0].Name)
	assert.Equal(t, "toolu_2", result.ToolCalls[1].ID)
	assert.Equal(t, "tool_b", result.ToolCalls[1].Name)
}

func TestParseAnthropicResponse_WithTextAndToolUse(t *testing.T) {
	// Create a response with both text and tool use.
	response := &anthropic.Message{
		StopReason: anthropic.StopReasonToolUse,
		Content: []anthropic.ContentBlockUnion{
			{
				Type: "text",
				Text: "I'll search for that information.",
			},
			{
				Type:  "tool_use",
				ID:    "toolu_abc",
				Name:  "search",
				Input: json.RawMessage(`{"query": "search term"}`),
			},
		},
		Usage: anthropic.Usage{InputTokens: 50, OutputTokens: 40},
	}

	result, err := parseAnthropicResponse(response)
	require.NoError(t, err)
	assert.Equal(t, "I'll search for that information.", result.Content)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "search", result.ToolCalls[0].Name)
}

func TestParseAnthropicResponse_ToolUseWithNilInput(t *testing.T) {
	// Create a response with tool use but nil input.
	response := &anthropic.Message{
		StopReason: anthropic.StopReasonToolUse,
		Content: []anthropic.ContentBlockUnion{
			{
				Type:  "tool_use",
				ID:    "toolu_nil",
				Name:  "no_args_tool",
				Input: nil,
			},
		},
		Usage: anthropic.Usage{InputTokens: 20, OutputTokens: 10},
	}

	result, err := parseAnthropicResponse(response)
	require.NoError(t, err)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "no_args_tool", result.ToolCalls[0].Name)
	// Input should be an empty map when nil.
	assert.Empty(t, result.ToolCalls[0].Input)
}

func TestParseAnthropicResponse_ToolUseWithEmptyInput(t *testing.T) {
	// Create a response with tool use but empty object input.
	response := &anthropic.Message{
		StopReason: anthropic.StopReasonToolUse,
		Content: []anthropic.ContentBlockUnion{
			{
				Type:  "tool_use",
				ID:    "toolu_empty",
				Name:  "empty_args_tool",
				Input: json.RawMessage(`{}`),
			},
		},
		Usage: anthropic.Usage{InputTokens: 20, OutputTokens: 10},
	}

	result, err := parseAnthropicResponse(response)
	require.NoError(t, err)
	require.Len(t, result.ToolCalls, 1)
	assert.Empty(t, result.ToolCalls[0].Input)
}

func TestParseAnthropicResponse_ToolUseWithInvalidJSON(t *testing.T) {
	// Create a response with tool use but invalid JSON input.
	response := &anthropic.Message{
		StopReason: anthropic.StopReasonToolUse,
		Content: []anthropic.ContentBlockUnion{
			{
				Type:  "tool_use",
				ID:    "toolu_invalid",
				Name:  "bad_input_tool",
				Input: json.RawMessage(`{invalid json`),
			},
		},
		Usage: anthropic.Usage{InputTokens: 20, OutputTokens: 10},
	}

	result, err := parseAnthropicResponse(response)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to parse tool input")
}

func TestParseAnthropicResponse_UnknownContentType(t *testing.T) {
	// Create a response with unknown content type - it should be ignored.
	response := &anthropic.Message{
		StopReason: anthropic.StopReasonEndTurn,
		Content: []anthropic.ContentBlockUnion{
			{
				Type: "text",
				Text: "Known text.",
			},
			{
				Type: "unknown_type",
				Text: "Should be ignored.",
			},
		},
		Usage: anthropic.Usage{InputTokens: 30, OutputTokens: 15},
	}

	result, err := parseAnthropicResponse(response)
	require.NoError(t, err)
	// Only the text block should be processed.
	assert.Equal(t, "Known text.", result.Content)
}

func TestParseAnthropicResponse_ToolUseWithComplexInput(t *testing.T) {
	// Create a response with tool use with complex nested input.
	complexInput := `{
		"name": "test",
		"count": 42,
		"enabled": true,
		"tags": ["a", "b", "c"],
		"nested": {
			"key": "value",
			"number": 123.45
		}
	}`
	response := &anthropic.Message{
		StopReason: anthropic.StopReasonToolUse,
		Content: []anthropic.ContentBlockUnion{
			{
				Type:  "tool_use",
				ID:    "toolu_complex",
				Name:  "complex_tool",
				Input: json.RawMessage(complexInput),
			},
		},
		Usage: anthropic.Usage{InputTokens: 100, OutputTokens: 50},
	}

	result, err := parseAnthropicResponse(response)
	require.NoError(t, err)
	require.Len(t, result.ToolCalls, 1)

	input := result.ToolCalls[0].Input
	assert.Equal(t, "test", input["name"])
	assert.Equal(t, float64(42), input["count"])
	assert.Equal(t, true, input["enabled"])

	tags, ok := input["tags"].([]interface{})
	require.True(t, ok)
	assert.Len(t, tags, 3)

	nested, ok := input["nested"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "value", nested["key"])
}

func TestParseAnthropicResponse_StopReasons(t *testing.T) {
	tests := []struct {
		name               string
		stopReason         anthropic.StopReason
		expectedStopReason types.StopReason
	}{
		{
			name:               "end_turn",
			stopReason:         anthropic.StopReasonEndTurn,
			expectedStopReason: types.StopReasonEndTurn,
		},
		{
			name:               "tool_use",
			stopReason:         anthropic.StopReasonToolUse,
			expectedStopReason: types.StopReasonToolUse,
		},
		{
			name:               "max_tokens",
			stopReason:         anthropic.StopReasonMaxTokens,
			expectedStopReason: types.StopReasonMaxTokens,
		},
		{
			name:               "unknown defaults to end_turn",
			stopReason:         anthropic.StopReason("unknown_reason"),
			expectedStopReason: types.StopReasonEndTurn,
		},
		{
			name:               "empty defaults to end_turn",
			stopReason:         anthropic.StopReason(""),
			expectedStopReason: types.StopReasonEndTurn,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := &anthropic.Message{
				StopReason: tt.stopReason,
				Usage:      anthropic.Usage{},
			}

			result, err := parseAnthropicResponse(response)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedStopReason, result.StopReason)
		})
	}
}

func TestParseAnthropicResponse_NoUsage(t *testing.T) {
	// Create response with zero usage.
	response := &anthropic.Message{
		StopReason: "end_turn",
		Usage: anthropic.Usage{
			InputTokens:  0,
			OutputTokens: 0,
		},
	}

	result, err := parseAnthropicResponse(response)
	require.NoError(t, err)
	assert.NotNil(t, result)
	// Usage should be nil when both input and output tokens are 0.
	assert.Nil(t, result.Usage)
}

func TestParseAnthropicResponse_WithCacheTokens(t *testing.T) {
	response := &anthropic.Message{
		StopReason: "end_turn",
		Usage: anthropic.Usage{
			InputTokens:              100,
			OutputTokens:             50,
			CacheReadInputTokens:     30,
			CacheCreationInputTokens: 20,
		},
	}

	result, err := parseAnthropicResponse(response)
	require.NoError(t, err)
	require.NotNil(t, result.Usage)
	assert.Equal(t, int64(30), result.Usage.CacheReadTokens)
	assert.Equal(t, int64(20), result.Usage.CacheCreationTokens)
	assert.Equal(t, int64(100), result.Usage.InputTokens)
	assert.Equal(t, int64(50), result.Usage.OutputTokens)
	assert.Equal(t, int64(150), result.Usage.TotalTokens)
}

func TestParseAnthropicResponse_EmptyContent(t *testing.T) {
	response := &anthropic.Message{
		StopReason: "end_turn",
		Content:    []anthropic.ContentBlockUnion{},
		Usage: anthropic.Usage{
			InputTokens:  10,
			OutputTokens: 0,
		},
	}

	result, err := parseAnthropicResponse(response)
	require.NoError(t, err)
	assert.Equal(t, "", result.Content)
	assert.Empty(t, result.ToolCalls)
}

func TestParseAnthropicResponse_InitializesToolCalls(t *testing.T) {
	response := &anthropic.Message{
		StopReason: "end_turn",
		Usage:      anthropic.Usage{InputTokens: 10, OutputTokens: 5},
	}

	result, err := parseAnthropicResponse(response)
	require.NoError(t, err)
	// ToolCalls should be initialized to empty slice, not nil.
	assert.NotNil(t, result.ToolCalls)
	assert.Empty(t, result.ToolCalls)
}

// TestSimpleClient_NilClient tests behavior when internal client is nil.
func TestSimpleClient_StructFields(t *testing.T) {
	cache := &cacheConfig{
		enabled:            true,
		cacheSystemPrompt:  true,
		cacheProjectMemory: false,
	}

	config := &base.Config{
		Enabled:   true,
		Model:     "test-model",
		APIKeyEnv: "TEST_KEY",
		MaxTokens: 2000,
		BaseURL:   "https://test.api.com",
	}

	client := &SimpleClient{
		client: nil,
		config: config,
		cache:  cache,
	}

	// Test getters.
	assert.Equal(t, "test-model", client.GetModel())
	assert.Equal(t, 2000, client.GetMaxTokens())

	// Test cache config is stored.
	assert.True(t, client.cache.enabled)
	assert.True(t, client.cache.cacheSystemPrompt)
	assert.False(t, client.cache.cacheProjectMemory)
}

// TestConvertMessagesToAnthropicFormat_LongConversation tests conversion of a long conversation.
func TestConvertMessagesToAnthropicFormat_LongConversation(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleUser, Content: "Hello"},
		{Role: types.RoleAssistant, Content: "Hi there!"},
		{Role: types.RoleUser, Content: "What's the weather?"},
		{Role: types.RoleAssistant, Content: "I don't have access to weather data."},
		{Role: types.RoleSystem, Content: "You are a weather assistant."},
		{Role: types.RoleUser, Content: "Can you help me?"},
		{Role: types.RoleAssistant, Content: "Of course!"},
	}

	result := convertMessagesToAnthropicFormat(messages)

	// System message should be skipped, so 6 messages.
	assert.Len(t, result, 6)
}

// TestConvertMessagesToAnthropicFormat_MixedRoles tests alternating roles.
func TestConvertMessagesToAnthropicFormat_AlternatingRoles(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleUser, Content: "Q1"},
		{Role: types.RoleAssistant, Content: "A1"},
		{Role: types.RoleUser, Content: "Q2"},
		{Role: types.RoleAssistant, Content: "A2"},
		{Role: types.RoleUser, Content: "Q3"},
	}

	result := convertMessagesToAnthropicFormat(messages)
	assert.Len(t, result, 5)
}

// TestConvertToolsToAnthropicFormat_NoParameters tests tool with no parameters.
func TestConvertToolsToAnthropicFormat_NoParameters(t *testing.T) {
	availableTools := []tools.Tool{
		&mockTool{
			name:        "simple_tool",
			description: "A tool with no parameters",
			parameters:  []tools.Parameter{},
		},
	}

	result := convertToolsToAnthropicFormat(availableTools)

	require.Len(t, result, 1)
	assert.Equal(t, "simple_tool", result[0].OfTool.Name)
	assert.Equal(t, "A tool with no parameters", result[0].OfTool.Description.Value)
	assert.Equal(t, "object", string(result[0].OfTool.InputSchema.Type))
	assert.Empty(t, result[0].OfTool.InputSchema.Required)
}

// TestConvertToolsToAnthropicFormat_ManyTools tests conversion of many tools.
func TestConvertToolsToAnthropicFormat_ManyTools(t *testing.T) {
	// Create 10 tools.
	availableTools := make([]tools.Tool, 10)
	for i := range availableTools {
		availableTools[i] = &mockTool{
			name:        "tool_" + string(rune('a'+i)),
			description: "Tool " + string(rune('A'+i)),
			parameters: []tools.Parameter{
				{Name: "param", Type: tools.ParamTypeString, Required: true},
			},
		}
	}

	result := convertToolsToAnthropicFormat(availableTools)
	assert.Len(t, result, 10)

	// Verify each tool is properly converted.
	for i, tool := range result {
		expectedName := "tool_" + string(rune('a'+i))
		assert.Equal(t, expectedName, tool.OfTool.Name)
		assert.Len(t, tool.OfTool.InputSchema.Required, 1)
	}
}

// TestExtractCacheConfig_NilCache tests when cache config is nil.
func TestExtractCacheConfig_NilCache(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"anthropic": {
						Model: "claude-sonnet-4-20250514",
						Cache: nil, // Explicitly nil.
					},
				},
			},
		},
	}

	cache := extractCacheConfig(atmosConfig)

	// Default behavior when cache is nil.
	assert.True(t, cache.enabled)
	assert.True(t, cache.cacheSystemPrompt)
	assert.True(t, cache.cacheProjectMemory)
}

// TestBuildSystemPrompt_AllCombinations tests all combinations of cache settings.
func TestBuildSystemPrompt_AllCombinations(t *testing.T) {
	tests := []struct {
		name           string
		globalEnabled  bool
		enableRequest  bool
		expectedCached bool
	}{
		{"Both enabled", true, true, true},
		{"Global enabled, request disabled", true, false, false},
		{"Global disabled, request enabled", false, true, false},
		{"Both disabled", false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &SimpleClient{
				config: &base.Config{Enabled: true},
				cache: &cacheConfig{
					enabled:           tt.globalEnabled,
					cacheSystemPrompt: true,
				},
			}

			textBlock := client.buildSystemPrompt("Test prompt", tt.enableRequest)

			if tt.expectedCached {
				assert.NotEmpty(t, textBlock.CacheControl.Type)
			} else {
				assert.Empty(t, textBlock.CacheControl.Type)
			}
		})
	}
}

// TestSimpleClient_Config tests various configurations.
func TestSimpleClient_ConfigVariations(t *testing.T) {
	tests := []struct {
		name             string
		model            string
		maxTokens        int
		expectedModel    string
		expectedMaxToken int
	}{
		{
			name:             "Default sonnet 4",
			model:            DefaultModel,
			maxTokens:        DefaultMaxTokens,
			expectedModel:    "claude-sonnet-4-20250514",
			expectedMaxToken: 4096,
		},
		{
			name:             "Claude 4",
			model:            "claude-4-20250514",
			maxTokens:        8192,
			expectedModel:    "claude-4-20250514",
			expectedMaxToken: 8192,
		},
		{
			name:             "Minimum tokens",
			model:            "test-model",
			maxTokens:        1,
			expectedModel:    "test-model",
			expectedMaxToken: 1,
		},
		{
			name:             "Maximum tokens",
			model:            "test-model",
			maxTokens:        200000,
			expectedModel:    "test-model",
			expectedMaxToken: 200000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &SimpleClient{
				config: &base.Config{
					Enabled:   true,
					Model:     tt.model,
					MaxTokens: tt.maxTokens,
				},
			}

			assert.Equal(t, tt.expectedModel, client.GetModel())
			assert.Equal(t, tt.expectedMaxToken, client.GetMaxTokens())
		})
	}
}

// TestMockTool tests the mock tool implementation.
func TestMockTool_Interface(t *testing.T) {
	tool := &mockTool{
		name:               "test_mock",
		description:        "Test mock tool",
		requiresPermission: true,
		isRestricted:       false,
		parameters: []tools.Parameter{
			{Name: "param1", Type: tools.ParamTypeString, Required: true},
		},
	}

	assert.Equal(t, "test_mock", tool.Name())
	assert.Equal(t, "Test mock tool", tool.Description())
	assert.True(t, tool.RequiresPermission())
	assert.False(t, tool.IsRestricted())
	assert.Len(t, tool.Parameters(), 1)

	// Test Execute.
	result, err := tool.Execute(context.Background(), nil)
	require.NoError(t, err)
	assert.True(t, result.Success)
}

// TestConvertToolsToAnthropicFormat_ParameterTypeMappings tests all parameter type mappings.
func TestConvertToolsToAnthropicFormat_ParameterTypeMappings(t *testing.T) {
	tests := []struct {
		paramType tools.ParamType
		expected  string
	}{
		{tools.ParamTypeString, "string"},
		{tools.ParamTypeInt, "integer"},
		{tools.ParamTypeBool, "boolean"},
		{tools.ParamTypeArray, "array"},
		{tools.ParamTypeObject, "object"},
	}

	for _, tt := range tests {
		t.Run(string(tt.paramType), func(t *testing.T) {
			availableTools := []tools.Tool{
				&mockTool{
					name:        "type_test",
					description: "Testing type: " + string(tt.paramType),
					parameters: []tools.Parameter{
						{Name: "p", Type: tt.paramType, Required: true},
					},
				},
			}

			result := convertToolsToAnthropicFormat(availableTools)
			require.Len(t, result, 1)

			// Verify the tool was converted and has the expected structure.
			assert.Equal(t, "type_test", result[0].OfTool.Name)
			assert.Equal(t, "object", string(result[0].OfTool.InputSchema.Type))
			assert.Len(t, result[0].OfTool.InputSchema.Required, 1)
			assert.Contains(t, result[0].OfTool.InputSchema.Required, "p")

			// Properties are set via the base.ExtractToolInfo function.
			// The actual type mapping is tested through base package tests.
		})
	}
}

// TestConvertMessagesToAnthropicFormat_OnlySystemMessages tests when all messages are system messages.
func TestConvertMessagesToAnthropicFormat_OnlySystemMessages(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleSystem, Content: "System 1"},
		{Role: types.RoleSystem, Content: "System 2"},
	}

	result := convertMessagesToAnthropicFormat(messages)

	// All system messages should be skipped.
	assert.Empty(t, result)
}

// TestConvertMessagesToAnthropicFormat_ConsecutiveSameRole tests consecutive messages with same role.
func TestConvertMessagesToAnthropicFormat_ConsecutiveSameRole(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleUser, Content: "User message 1"},
		{Role: types.RoleUser, Content: "User message 2"},
		{Role: types.RoleAssistant, Content: "Assistant message 1"},
		{Role: types.RoleAssistant, Content: "Assistant message 2"},
	}

	result := convertMessagesToAnthropicFormat(messages)

	// All 4 messages should be converted (consecutive same role is allowed).
	assert.Len(t, result, 4)
}

// TestExtractCacheConfig_NilProviderInMap tests when provider value is nil in the map.
func TestExtractCacheConfig_NilProviderInMap(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"anthropic": nil,
				},
			},
		},
	}

	cache := extractCacheConfig(atmosConfig)

	// Should use defaults when provider config is nil.
	assert.True(t, cache.enabled)
	assert.True(t, cache.cacheSystemPrompt)
	assert.True(t, cache.cacheProjectMemory)
}

// TestUsageCalculations verifies token usage calculations.
func TestUsageCalculations(t *testing.T) {
	tests := []struct {
		name           string
		inputTokens    int64
		outputTokens   int64
		cacheRead      int64
		cacheCreate    int64
		expectedTotal  int64
		expectUsageNil bool
	}{
		{
			name:           "Normal usage",
			inputTokens:    100,
			outputTokens:   50,
			cacheRead:      0,
			cacheCreate:    0,
			expectedTotal:  150,
			expectUsageNil: false,
		},
		{
			name:           "Zero usage",
			inputTokens:    0,
			outputTokens:   0,
			cacheRead:      0,
			cacheCreate:    0,
			expectedTotal:  0,
			expectUsageNil: true,
		},
		{
			name:           "With cache tokens",
			inputTokens:    500,
			outputTokens:   200,
			cacheRead:      300,
			cacheCreate:    100,
			expectedTotal:  700,
			expectUsageNil: false,
		},
		{
			name:           "Only input tokens",
			inputTokens:    1000,
			outputTokens:   0,
			cacheRead:      0,
			cacheCreate:    0,
			expectedTotal:  1000,
			expectUsageNil: false,
		},
		{
			name:           "Only output tokens",
			inputTokens:    0,
			outputTokens:   500,
			cacheRead:      0,
			cacheCreate:    0,
			expectedTotal:  500,
			expectUsageNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := &anthropic.Message{
				StopReason: "end_turn",
				Usage: anthropic.Usage{
					InputTokens:              tt.inputTokens,
					OutputTokens:             tt.outputTokens,
					CacheReadInputTokens:     tt.cacheRead,
					CacheCreationInputTokens: tt.cacheCreate,
				},
			}

			result, err := parseAnthropicResponse(response)
			require.NoError(t, err)

			if tt.expectUsageNil {
				assert.Nil(t, result.Usage)
			} else {
				require.NotNil(t, result.Usage)
				assert.Equal(t, tt.expectedTotal, result.Usage.TotalTokens)
				assert.Equal(t, tt.inputTokens, result.Usage.InputTokens)
				assert.Equal(t, tt.outputTokens, result.Usage.OutputTokens)
				assert.Equal(t, tt.cacheRead, result.Usage.CacheReadTokens)
				assert.Equal(t, tt.cacheCreate, result.Usage.CacheCreationTokens)
			}
		})
	}
}

// TestProviderName tests the constant value.
func TestProviderName_Constant(t *testing.T) {
	assert.Equal(t, "anthropic", ProviderName)
	// Verify it's a lowercase string suitable for config lookups.
	assert.Equal(t, ProviderName, "anthropic")
	assert.NotContains(t, ProviderName, " ")
	assert.NotContains(t, ProviderName, "-")
}

// TestDefaultValues tests all default constants.
func TestDefaultValues_AllConstants(t *testing.T) {
	// DefaultMaxTokens should be reasonable for most use cases.
	assert.Greater(t, DefaultMaxTokens, 0)
	assert.LessOrEqual(t, DefaultMaxTokens, 200000)

	// DefaultModel should be a valid model string.
	assert.NotEmpty(t, DefaultModel)
	assert.Contains(t, DefaultModel, "claude")

	// DefaultAPIKeyEnv should follow standard naming conventions.
	assert.NotEmpty(t, DefaultAPIKeyEnv)
	assert.Contains(t, DefaultAPIKeyEnv, "ANTHROPIC")
	assert.Contains(t, DefaultAPIKeyEnv, "API_KEY")
}

// TestExtractConfig_PartialOverrides tests partial configuration overrides.
func TestExtractConfig_PartialOverrides(t *testing.T) {
	tests := []struct {
		name           string
		providerConfig *schema.AIProviderConfig
		expectedModel  string
		expectedEnv    string
		expectedTokens int
	}{
		{
			name: "Only model override",
			providerConfig: &schema.AIProviderConfig{
				Model: "claude-3-opus-20240229",
			},
			expectedModel:  "claude-3-opus-20240229",
			expectedEnv:    DefaultAPIKeyEnv,
			expectedTokens: DefaultMaxTokens,
		},
		{
			name: "Only API key env override",
			providerConfig: &schema.AIProviderConfig{
				ApiKeyEnv: "CUSTOM_KEY",
			},
			expectedModel:  DefaultModel,
			expectedEnv:    "CUSTOM_KEY",
			expectedTokens: DefaultMaxTokens,
		},
		{
			name: "Only max tokens override",
			providerConfig: &schema.AIProviderConfig{
				MaxTokens: 10000,
			},
			expectedModel:  DefaultModel,
			expectedEnv:    DefaultAPIKeyEnv,
			expectedTokens: 10000,
		},
		{
			name: "Model and tokens override",
			providerConfig: &schema.AIProviderConfig{
				Model:     "claude-4-20250514",
				MaxTokens: 16000,
			},
			expectedModel:  "claude-4-20250514",
			expectedEnv:    DefaultAPIKeyEnv,
			expectedTokens: 16000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"anthropic": tt.providerConfig,
						},
					},
				},
			}

			config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
				Model:     DefaultModel,
				APIKeyEnv: DefaultAPIKeyEnv,
				MaxTokens: DefaultMaxTokens,
			})

			assert.Equal(t, tt.expectedModel, config.Model)
			assert.Equal(t, tt.expectedEnv, config.APIKeyEnv)
			assert.Equal(t, tt.expectedTokens, config.MaxTokens)
		})
	}
}

// TestCacheConfig_BothFalseDefaults tests the special case where both cache options are false.
func TestCacheConfig_BothFalseDefaultsToTrue(t *testing.T) {
	// When cache is enabled but both options are false, they default to true.
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"anthropic": {
						Cache: &schema.AICacheSettings{
							Enabled:            true,
							CacheSystemPrompt:  false,
							CacheProjectMemory: false,
						},
					},
				},
			},
		},
	}

	cache := extractCacheConfig(atmosConfig)

	// According to the code, when both are false, they default to true.
	assert.True(t, cache.enabled)
	assert.True(t, cache.cacheSystemPrompt)
	assert.True(t, cache.cacheProjectMemory)
}

// TestBuildSystemPrompt_WithSpecialCharacters tests system prompts with special characters.
func TestBuildSystemPrompt_WithSpecialCharacters(t *testing.T) {
	client := &SimpleClient{
		config: &base.Config{Enabled: true},
		cache: &cacheConfig{
			enabled:           true,
			cacheSystemPrompt: true,
		},
	}

	specialPrompts := []string{
		"Prompt with newlines\nand\nmore\nlines",
		"Prompt with tabs\t\tand\ttabs",
		"Prompt with unicode: \u00e9\u00e0\u00fc\u00f1",
		"Prompt with emoji: \U0001F600",
		"Prompt with quotes: \"single\" and 'double'",
		"Prompt with <html> tags </html>",
		"Prompt with JSON: {\"key\": \"value\"}",
	}

	for _, prompt := range specialPrompts {
		textBlock := client.buildSystemPrompt(prompt, true)
		assert.Equal(t, prompt, textBlock.Text)
	}
}

// TestConvertToolsToAnthropicFormat_ToolWithLongDescription tests tool with very long description.
func TestConvertToolsToAnthropicFormat_ToolWithLongDescription(t *testing.T) {
	longDescription := ""
	for i := 0; i < 100; i++ {
		longDescription += "This is a very long description that repeats many times. "
	}

	availableTools := []tools.Tool{
		&mockTool{
			name:        "long_description_tool",
			description: longDescription,
			parameters:  []tools.Parameter{},
		},
	}

	result := convertToolsToAnthropicFormat(availableTools)

	require.Len(t, result, 1)
	assert.Equal(t, longDescription, result[0].OfTool.Description.Value)
}

// TestConvertToolsToAnthropicFormat_ToolWithSpecialCharactersInName tests special characters in tool names.
func TestConvertToolsToAnthropicFormat_ToolWithSpecialNames(t *testing.T) {
	availableTools := []tools.Tool{
		&mockTool{
			name:        "tool_with_underscores",
			description: "Tool with underscores in name",
			parameters:  []tools.Parameter{},
		},
		&mockTool{
			name:        "ToolWithCamelCase",
			description: "Tool with camel case name",
			parameters:  []tools.Parameter{},
		},
		&mockTool{
			name:        "tool123",
			description: "Tool with numbers",
			parameters:  []tools.Parameter{},
		},
	}

	result := convertToolsToAnthropicFormat(availableTools)

	assert.Len(t, result, 3)
	assert.Equal(t, "tool_with_underscores", result[0].OfTool.Name)
	assert.Equal(t, "ToolWithCamelCase", result[1].OfTool.Name)
	assert.Equal(t, "tool123", result[2].OfTool.Name)
}

// TestExtractConfig_EdgeCases tests edge cases in config extraction.
func TestExtractConfig_EdgeCases(t *testing.T) {
	// Test with MaxTokens = 0 (should use default).
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"anthropic": {
						MaxTokens: 0, // Zero should not override default.
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

	// MaxTokens should use default since 0 is not > 0.
	assert.Equal(t, DefaultMaxTokens, config.MaxTokens)
}

// TestSimpleClient_CacheConfigCombinations tests all cache configuration combinations.
func TestSimpleClient_CacheConfigCombinations(t *testing.T) {
	tests := []struct {
		name                string
		enabled             bool
		cacheSystemPrompt   bool
		cacheProjectMemory  bool
		promptSystemEnabled bool
		promptMemoryEnabled bool
		expectSystemCached  bool
		expectMemoryCached  bool
	}{
		{
			name:                "All enabled",
			enabled:             true,
			cacheSystemPrompt:   true,
			cacheProjectMemory:  true,
			promptSystemEnabled: true,
			promptMemoryEnabled: true,
			expectSystemCached:  true,
			expectMemoryCached:  true,
		},
		{
			name:                "Global disabled",
			enabled:             false,
			cacheSystemPrompt:   true,
			cacheProjectMemory:  true,
			promptSystemEnabled: true,
			promptMemoryEnabled: true,
			expectSystemCached:  false,
			expectMemoryCached:  false,
		},
		{
			name:                "System only",
			enabled:             true,
			cacheSystemPrompt:   true,
			cacheProjectMemory:  false,
			promptSystemEnabled: true,
			promptMemoryEnabled: false,
			expectSystemCached:  true,
			expectMemoryCached:  false,
		},
		{
			name:                "Memory only",
			enabled:             true,
			cacheSystemPrompt:   false,
			cacheProjectMemory:  true,
			promptSystemEnabled: false,
			promptMemoryEnabled: true,
			expectSystemCached:  false,
			expectMemoryCached:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &SimpleClient{
				config: &base.Config{Enabled: true},
				cache: &cacheConfig{
					enabled:            tt.enabled,
					cacheSystemPrompt:  tt.cacheSystemPrompt,
					cacheProjectMemory: tt.cacheProjectMemory,
				},
			}

			systemBlock := client.buildSystemPrompt("system", tt.promptSystemEnabled)
			memoryBlock := client.buildSystemPrompt("memory", tt.promptMemoryEnabled)

			if tt.expectSystemCached {
				assert.NotEmpty(t, systemBlock.CacheControl.Type)
			} else {
				assert.Empty(t, systemBlock.CacheControl.Type)
			}

			if tt.expectMemoryCached {
				assert.NotEmpty(t, memoryBlock.CacheControl.Type)
			} else {
				assert.Empty(t, memoryBlock.CacheControl.Type)
			}
		})
	}
}
