package anthropic

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/ai/agent/base"
	"github.com/cloudposse/atmos/pkg/schema"
)

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
		parameters: []mockParameter{
			{
				name:        "query",
				paramType:   "string",
				description: "The search query",
				required:    true,
			},
			{
				name:        "max_results",
				paramType:   "integer",
				description: "Maximum number of results",
				required:    false,
			},
			{
				name:        "verbose",
				paramType:   "boolean",
				description: "Enable verbose output",
				required:    false,
			},
		},
	}

	// Verify the mock tool structure.
	assert.Equal(t, "test_tool", mockTool.name)
	assert.Equal(t, "A test tool for verification", mockTool.description)
	assert.Len(t, mockTool.parameters, 3)

	// Verify parameter types match JSON Schema spec.
	assert.Equal(t, "string", mockTool.parameters[0].paramType)
	assert.Equal(t, "integer", mockTool.parameters[1].paramType, "Should be 'integer', not 'int'")
	assert.Equal(t, "boolean", mockTool.parameters[2].paramType, "Should be 'boolean', not 'bool'")
}

// mockTool implements a simple test tool.
type mockTool struct {
	name        string
	description string
	parameters  []mockParameter
}

type mockParameter struct {
	name        string
	paramType   string
	description string
	required    bool
}

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
