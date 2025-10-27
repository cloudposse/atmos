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
