package openai

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
				Model:     "gpt-4o",
				APIKeyEnv: "OPENAI_API_KEY",
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
							"openai": {
								Model:     "gpt-4-turbo",
								ApiKeyEnv: "CUSTOM_API_KEY",
								MaxTokens: 8192,
							},
						},
					},
				},
			},
			expectedConfig: &base.Config{
				Enabled:   true,
				Model:     "gpt-4-turbo",
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
							"openai": {
								Model: "gpt-3.5-turbo",
							},
						},
					},
				},
			},
			expectedConfig: &base.Config{
				Enabled:   true,
				Model:     "gpt-3.5-turbo",
				APIKeyEnv: "OPENAI_API_KEY",
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
	config := &base.Config{
		Enabled:   true,
		Model:     "gpt-4o",
		APIKeyEnv: "OPENAI_API_KEY",
		MaxTokens: 4096,
	}

	client := &Client{
		client: nil, // We don't need a real client for testing getters.
		config: config,
	}

	assert.Equal(t, "gpt-4o", client.GetModel())
	assert.Equal(t, 4096, client.GetMaxTokens())
}
