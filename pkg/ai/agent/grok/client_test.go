package grok

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExtractConfig(t *testing.T) {
	tests := []struct {
		name           string
		atmosConfig    *schema.AtmosConfiguration
		expectedConfig *Config
	}{
		{
			name: "Default configuration",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{},
				},
			},
			expectedConfig: &Config{
				Enabled:   false,
				Model:     "grok-beta",
				APIKeyEnv: "XAI_API_KEY",
				MaxTokens: 4096,
				BaseURL:   "https://api.x.ai/v1",
			},
		},
		{
			name: "Enabled configuration",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled:   true,
						Model:     "grok-4",
						ApiKeyEnv: "CUSTOM_XAI_KEY",
						MaxTokens: 8192,
						BaseURL:   "https://custom.api.x.ai/v1",
					},
				},
			},
			expectedConfig: &Config{
				Enabled:   true,
				Model:     "grok-4",
				APIKeyEnv: "CUSTOM_XAI_KEY",
				MaxTokens: 8192,
				BaseURL:   "https://custom.api.x.ai/v1",
			},
		},
		{
			name: "Partial configuration",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Model:   "grok-2",
					},
				},
			},
			expectedConfig: &Config{
				Enabled:   true,
				Model:     "grok-2",
				APIKeyEnv: "XAI_API_KEY",
				MaxTokens: 4096,
				BaseURL:   "https://api.x.ai/v1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := extractConfig(tt.atmosConfig)
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

func TestClientGetters(t *testing.T) {
	config := &Config{
		Enabled:   true,
		Model:     "grok-beta",
		APIKeyEnv: "XAI_API_KEY",
		MaxTokens: 4096,
		BaseURL:   "https://api.x.ai/v1",
	}

	client := &Client{
		client: nil, // We don't need a real client for testing getters.
		config: config,
	}

	assert.Equal(t, "grok-beta", client.GetModel())
	assert.Equal(t, 4096, client.GetMaxTokens())
	assert.Equal(t, "https://api.x.ai/v1", client.GetBaseURL())
}
