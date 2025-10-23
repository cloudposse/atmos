package gemini

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
						Enabled:   true,
						Model:     "gemini-1.5-pro",
						ApiKeyEnv: "CUSTOM_GEMINI_KEY",
						MaxTokens: 16384,
					},
				},
			},
			expectedConfig: &Config{
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
						Model:   "gemini-1.5-flash",
					},
				},
			},
			expectedConfig: &Config{
				Enabled:   true,
				Model:     "gemini-1.5-flash",
				APIKeyEnv: "GEMINI_API_KEY",
				MaxTokens: 8192,
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

	client, err := NewClient(nil, atmosConfig)
	assert.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "AI features are disabled")
}

func TestClientGetters(t *testing.T) {
	config := &Config{
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
