package ollama

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
				Model:     "llama3.3:70b",
				APIKeyEnv: "OLLAMA_API_KEY",
				MaxTokens: 4096,
				BaseURL:   "http://localhost:11434/v1",
			},
		},
		{
			name: "Enabled configuration",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"ollama": {
								Model:     "llama3.1:8b",
								ApiKeyEnv: "CUSTOM_API_KEY",
								MaxTokens: 8192,
								BaseURL:   "http://custom-ollama:11434/v1",
							},
						},
					},
				},
			},
			expectedConfig: &Config{
				Enabled:   true,
				Model:     "llama3.1:8b",
				APIKeyEnv: "CUSTOM_API_KEY",
				MaxTokens: 8192,
				BaseURL:   "http://custom-ollama:11434/v1",
			},
		},
		{
			name: "Partial configuration",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"ollama": {
								Model: "codellama:13b",
							},
						},
					},
				},
			},
			expectedConfig: &Config{
				Enabled:   true,
				Model:     "codellama:13b",
				APIKeyEnv: "OLLAMA_API_KEY",
				MaxTokens: 4096,
				BaseURL:   "http://localhost:11434/v1",
			},
		},
		{
			name: "Custom base URL only",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"ollama": {
								BaseURL: "https://ollama.example.com/v1",
							},
						},
					},
				},
			},
			expectedConfig: &Config{
				Enabled:   true,
				Model:     "llama3.3:70b",
				APIKeyEnv: "OLLAMA_API_KEY",
				MaxTokens: 4096,
				BaseURL:   "https://ollama.example.com/v1",
			},
		},
		{
			name: "Remote Ollama with API key",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"ollama": {
								Model:     "llama3.3:70b",
								ApiKeyEnv: "OLLAMA_REMOTE_KEY",
								BaseURL:   "https://api.ollama-cloud.com/v1",
								MaxTokens: 16384,
							},
						},
					},
				},
			},
			expectedConfig: &Config{
				Enabled:   true,
				Model:     "llama3.3:70b",
				APIKeyEnv: "OLLAMA_REMOTE_KEY",
				MaxTokens: 16384,
				BaseURL:   "https://api.ollama-cloud.com/v1",
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
		Model:     "llama3.3:70b",
		APIKeyEnv: "OLLAMA_API_KEY",
		MaxTokens: 4096,
		BaseURL:   "http://localhost:11434/v1",
	}

	client := &Client{
		client: nil, // We don't need a real client for testing getters
		config: config,
	}

	assert.Equal(t, "llama3.3:70b", client.GetModel())
	assert.Equal(t, 4096, client.GetMaxTokens())
}

func TestDefaultConstants(t *testing.T) {
	assert.Equal(t, 4096, DefaultMaxTokens)
	assert.Equal(t, "llama3.3:70b", DefaultModel)
	assert.Equal(t, "http://localhost:11434/v1", DefaultBaseURL)
	assert.Equal(t, "OLLAMA_API_KEY", DefaultAPIKeyEnv)
}

func TestConfig_AllFields(t *testing.T) {
	config := &Config{
		Enabled:   true,
		Model:     "test-model",
		APIKeyEnv: "TEST_KEY",
		MaxTokens: 1000,
		BaseURL:   "http://test:1234/v1",
	}

	assert.True(t, config.Enabled)
	assert.Equal(t, "test-model", config.Model)
	assert.Equal(t, "TEST_KEY", config.APIKeyEnv)
	assert.Equal(t, 1000, config.MaxTokens)
	assert.Equal(t, "http://test:1234/v1", config.BaseURL)
}
