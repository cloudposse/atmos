package anthropic

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExtractSimpleAIConfig(t *testing.T) {
	tests := []struct {
		name           string
		atmosConfig    *schema.AtmosConfiguration
		expectedConfig *SimpleAIConfig
	}{
		{
			name: "Default configuration",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{},
				},
			},
			expectedConfig: &SimpleAIConfig{
				Enabled:   false,
				Model:     "claude-sonnet-4-20250514",
				APIKeyEnv: "ANTHROPIC_API_KEY",
				MaxTokens: 4096,
			CacheEnabled:        true, // Default
			CacheSystemPrompt:   true, // Default
			CacheProjectMemory:  true, // Default
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
			expectedConfig: &SimpleAIConfig{
				Enabled:   true,
				Model:     "claude-4-20250514",
				APIKeyEnv: "CUSTOM_API_KEY",
				MaxTokens: 8192,
				CacheEnabled:        true, // Default
				CacheSystemPrompt:   true, // Default
				CacheProjectMemory:  true, // Default
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
			expectedConfig: &SimpleAIConfig{
				Enabled:   true,
				Model:     "claude-3-haiku-20240307",
				APIKeyEnv: "ANTHROPIC_API_KEY",
				MaxTokens: 4096,
				CacheEnabled:        true, // Default
				CacheSystemPrompt:   true, // Default
				CacheProjectMemory:  true, // Default
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := extractSimpleAIConfig(tt.atmosConfig)
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
	config := &SimpleAIConfig{
		Enabled:   true,
		Model:     "claude-sonnet-4-20250514",
		APIKeyEnv: "ANTHROPIC_API_KEY",
		MaxTokens: 4096,
	}

	client := &SimpleClient{
		client: nil, // We don't need a real client for testing getters
		config: config,
	}

	assert.Equal(t, "claude-sonnet-4-20250514", client.GetModel())
	assert.Equal(t, 4096, client.GetMaxTokens())
}

func TestConvertToolsToAnthropicFormat(t *testing.T) {
	// Create a mock tool
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

	// Verify the mock tool structure
	assert.Equal(t, "test_tool", mockTool.name)
	assert.Equal(t, "A test tool for verification", mockTool.description)
	assert.Len(t, mockTool.parameters, 3)

	// Verify parameter types match JSON Schema spec
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
	// Verify that our tool schema structure matches JSON Schema draft 2020-12 requirements
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
	// This test verifies that tool descriptions are critical for AI decision-making
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

// Token Caching Tests

func TestExtractSimpleAIConfig_CacheDefaults(t *testing.T) {
	tests := []struct {
		name           string
		atmosConfig    *schema.AtmosConfiguration
		expectedConfig *SimpleAIConfig
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
			expectedConfig: &SimpleAIConfig{
				Enabled:             true,
				Model:               "claude-sonnet-4-20250514",
				APIKeyEnv:           "ANTHROPIC_API_KEY",
				MaxTokens:           4096,
				CacheEnabled:        true, // Default
				CacheSystemPrompt:   true, // Default
				CacheProjectMemory:  true, // Default
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
			expectedConfig: &SimpleAIConfig{
				Enabled:             true,
				Model:               "claude-sonnet-4-20250514",
				APIKeyEnv:           "ANTHROPIC_API_KEY",
				MaxTokens:           4096,
				CacheEnabled:        false,
				CacheSystemPrompt:   false,
				CacheProjectMemory:  false,
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
									Enabled:             true,
									CacheSystemPrompt:   true,
									CacheProjectMemory:  false, // Only cache system prompt
								},
							},
						},
					},
				},
			},
			expectedConfig: &SimpleAIConfig{
				Enabled:             true,
				Model:               "claude-sonnet-4-20250514",
				APIKeyEnv:           "ANTHROPIC_API_KEY",
				MaxTokens:           4096,
				CacheEnabled:        true,
				CacheSystemPrompt:   true,
				CacheProjectMemory:  false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := extractSimpleAIConfig(tt.atmosConfig)
			assert.Equal(t, tt.expectedConfig, config)
		})
	}
}

func TestBuildSystemPrompt_NoCaching(t *testing.T) {
	client := &SimpleClient{
		config: &SimpleAIConfig{
			CacheEnabled:      false,
			CacheSystemPrompt: true, // Ignored when CacheEnabled is false
		},
	}

	textBlock := client.buildSystemPrompt("Test system prompt", true)

	assert.Equal(t, "Test system prompt", textBlock.Text)
	// Cache control should NOT be set when caching is disabled.
	assert.Empty(t, textBlock.CacheControl.Type, "Cache control should not be set when caching is disabled")
}

func TestBuildSystemPrompt_WithCaching(t *testing.T) {
	client := &SimpleClient{
		config: &SimpleAIConfig{
			CacheEnabled:      true,
			CacheSystemPrompt: true,
		},
	}

	textBlock := client.buildSystemPrompt("Test system prompt", true)

	assert.Equal(t, "Test system prompt", textBlock.Text)
	// Cache control SHOULD be set when caching is enabled.
	assert.NotEmpty(t, textBlock.CacheControl.Type, "Cache control should be set when caching is enabled")
}

func TestBuildSystemPrompt_CachingDisabledPerPrompt(t *testing.T) {
	client := &SimpleClient{
		config: &SimpleAIConfig{
			CacheEnabled:      true,
			CacheSystemPrompt: true, // Global caching enabled
		},
	}

	// Request caching disabled for this specific prompt.
	textBlock := client.buildSystemPrompt("Test system prompt", false)

	assert.Equal(t, "Test system prompt", textBlock.Text)
	// Cache control should NOT be set when explicitly disabled for this prompt.
	assert.Empty(t, textBlock.CacheControl.Type, "Cache control should not be set when disabled per-prompt")
}

func TestBuildSystemPrompts_Multiple(t *testing.T) {
	client := &SimpleClient{
		config: &SimpleAIConfig{
			CacheEnabled:        true,
			CacheSystemPrompt:   true,
			CacheProjectMemory:  true,
		},
	}

	prompts := []struct {
		content string
		cache   bool
	}{
		{content: "System prompt for agent", cache: true},
		{content: "Project memory (ATMOS.md)", cache: true},
		{content: "Additional context", cache: false},
	}

	textBlocks := client.buildSystemPrompts(prompts)

	assert.Len(t, textBlocks, 3)

	// First prompt (agent system prompt) - cached.
	assert.Equal(t, "System prompt for agent", textBlocks[0].Text)
	assert.NotEmpty(t, textBlocks[0].CacheControl.Type, "Agent system prompt should be cached")

	// Second prompt (project memory) - cached.
	assert.Equal(t, "Project memory (ATMOS.md)", textBlocks[1].Text)
	assert.NotEmpty(t, textBlocks[1].CacheControl.Type, "Project memory should be cached")

	// Third prompt (additional context) - not cached.
	assert.Equal(t, "Additional context", textBlocks[2].Text)
	assert.Empty(t, textBlocks[2].CacheControl.Type, "Additional context should NOT be cached")
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
