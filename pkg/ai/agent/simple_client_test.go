package agent

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
					AI: nil,
				},
			},
			expectedConfig: &SimpleAIConfig{
				Enabled:   false,
				Model:     "claude-3-5-sonnet-20241022",
				APIKeyEnv: "ANTHROPIC_API_KEY",
				MaxTokens: 4096,
			},
		},
		{
			name: "Enabled configuration",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: map[string]interface{}{
						"enabled":     true,
						"model":       "claude-4-20250514",
						"api_key_env": "CUSTOM_API_KEY",
						"max_tokens":  8192,
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
					AI: map[string]interface{}{
						"enabled": true,
						"model":   "claude-3-haiku-20240307",
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
			AI: map[string]interface{}{
				"enabled": false,
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
		Model:     "claude-3-5-sonnet-20241022",
		APIKeyEnv: "ANTHROPIC_API_KEY",
		MaxTokens: 4096,
	}

	client := &SimpleClient{
		client: nil, // We don't need a real client for testing getters
		config: config,
	}

	assert.Equal(t, "claude-3-5-sonnet-20241022", client.GetModel())
	assert.Equal(t, 4096, client.GetMaxTokens())
}
