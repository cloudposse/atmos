package azureopenai

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
				APIKeyEnv: "AZURE_OPENAI_API_KEY",
				MaxTokens: 4096,
				BaseURL:   "",
			},
		},
		{
			name: "Enabled configuration",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"azureopenai": {
								Model:     "gpt-4-turbo",
								ApiKeyEnv: "CUSTOM_AZURE_KEY",
								MaxTokens: 8192,
								BaseURL:   "https://myresource.openai.azure.com",
							},
						},
					},
				},
			},
			expectedConfig: &base.Config{
				Enabled:   true,
				Model:     "gpt-4-turbo",
				APIKeyEnv: "CUSTOM_AZURE_KEY",
				MaxTokens: 8192,
				BaseURL:   "https://myresource.openai.azure.com",
			},
		},
		{
			name: "Partial configuration with endpoint",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"azureopenai": {
								Model:   "gpt-35-turbo",
								BaseURL: "https://company.openai.azure.com",
							},
						},
					},
				},
			},
			expectedConfig: &base.Config{
				Enabled:   true,
				Model:     "gpt-35-turbo",
				APIKeyEnv: "AZURE_OPENAI_API_KEY",
				MaxTokens: 4096,
				BaseURL:   "https://company.openai.azure.com",
			},
		},
		{
			name: "Custom deployment name",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"azureopenai": {
								Model:   "my-gpt4-deployment",
								BaseURL: "https://prod-ai.openai.azure.com",
							},
						},
					},
				},
			},
			expectedConfig: &base.Config{
				Enabled:   true,
				Model:     "my-gpt4-deployment",
				APIKeyEnv: "AZURE_OPENAI_API_KEY",
				MaxTokens: 4096,
				BaseURL:   "https://prod-ai.openai.azure.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := base.ExtractConfig(tt.atmosConfig, ProviderName, base.ProviderDefaults{
				Model:     DefaultModel,
				APIKeyEnv: DefaultAPIKeyEnv,
				MaxTokens: DefaultMaxTokens,
				BaseURL:   "",
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
		APIKeyEnv: "AZURE_OPENAI_API_KEY",
		MaxTokens: 4096,
		BaseURL:   "https://myresource.openai.azure.com",
	}

	client := &Client{
		client:     nil, // We don't need a real client for testing getters.
		config:     config,
		apiVersion: "2024-02-15-preview",
	}

	assert.Equal(t, "gpt-4o", client.GetModel())
	assert.Equal(t, 4096, client.GetMaxTokens())
	assert.Equal(t, "https://myresource.openai.azure.com", client.GetBaseURL())
	assert.Equal(t, "2024-02-15-preview", client.GetAPIVersion())
}

func TestDefaultConstants(t *testing.T) {
	assert.Equal(t, 4096, DefaultMaxTokens)
	assert.Equal(t, "gpt-4o", DefaultModel)
	assert.Equal(t, "AZURE_OPENAI_API_KEY", DefaultAPIKeyEnv)
	assert.Equal(t, "2024-02-15-preview", DefaultAPIVersion)
}

func TestConfig_AllFields(t *testing.T) {
	config := &base.Config{
		Enabled:   true,
		Model:     "test-model",
		APIKeyEnv: "TEST_KEY",
		MaxTokens: 1000,
		BaseURL:   "https://test.openai.azure.com",
	}

	assert.True(t, config.Enabled)
	assert.Equal(t, "test-model", config.Model)
	assert.Equal(t, "TEST_KEY", config.APIKeyEnv)
	assert.Equal(t, 1000, config.MaxTokens)
	assert.Equal(t, "https://test.openai.azure.com", config.BaseURL)
}
