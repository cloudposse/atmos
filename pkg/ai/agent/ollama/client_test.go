package ollama

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
			expectedConfig: &base.Config{
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
			expectedConfig: &base.Config{
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
			expectedConfig: &base.Config{
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
			expectedConfig: &base.Config{
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
			config := base.ExtractConfig(tt.atmosConfig, ProviderName, base.ProviderDefaults{
				Model:     DefaultModel,
				APIKeyEnv: DefaultAPIKeyEnv,
				MaxTokens: DefaultMaxTokens,
				BaseURL:   DefaultBaseURL,
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
		Model:     "llama3.3:70b",
		APIKeyEnv: "OLLAMA_API_KEY",
		MaxTokens: 4096,
		BaseURL:   "http://localhost:11434/v1",
	}

	client := &Client{
		client: nil, // We don't need a real client for testing getters.
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
	config := &base.Config{
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

func TestClientGetters_GetBaseURL(t *testing.T) {
	config := &base.Config{
		Enabled:   true,
		Model:     "llama3.3:70b",
		APIKeyEnv: "OLLAMA_API_KEY",
		MaxTokens: 4096,
		BaseURL:   "http://localhost:11434/v1",
	}

	client := &Client{
		client: nil,
		config: config,
	}

	assert.Equal(t, "http://localhost:11434/v1", client.GetBaseURL())
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
		BaseURL:   DefaultBaseURL,
	})

	// Should use defaults when providers is nil.
	assert.True(t, config.Enabled)
	assert.Equal(t, DefaultModel, config.Model)
	assert.Equal(t, DefaultAPIKeyEnv, config.APIKeyEnv)
	assert.Equal(t, DefaultMaxTokens, config.MaxTokens)
	assert.Equal(t, DefaultBaseURL, config.BaseURL)
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
		BaseURL:   DefaultBaseURL,
	})

	// Should use defaults when this provider is not configured.
	assert.True(t, config.Enabled)
	assert.Equal(t, DefaultModel, config.Model)
	assert.Equal(t, DefaultAPIKeyEnv, config.APIKeyEnv)
	assert.Equal(t, DefaultMaxTokens, config.MaxTokens)
	assert.Equal(t, DefaultBaseURL, config.BaseURL)
}

func TestExtractConfig_NilProviderConfig(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"ollama": nil, // Explicitly nil provider config.
				},
			},
		},
	}

	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:     DefaultModel,
		APIKeyEnv: DefaultAPIKeyEnv,
		MaxTokens: DefaultMaxTokens,
		BaseURL:   DefaultBaseURL,
	})

	// Should use defaults when provider config is nil.
	assert.True(t, config.Enabled)
	assert.Equal(t, DefaultModel, config.Model)
	assert.Equal(t, DefaultAPIKeyEnv, config.APIKeyEnv)
	assert.Equal(t, DefaultMaxTokens, config.MaxTokens)
	assert.Equal(t, DefaultBaseURL, config.BaseURL)
}

func TestClientGetters_CustomValues(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		maxTokens int
		baseURL   string
	}{
		{
			name:      "Default Llama",
			model:     "llama3.3:70b",
			maxTokens: 4096,
			baseURL:   "http://localhost:11434/v1",
		},
		{
			name:      "CodeLlama",
			model:     "codellama:13b",
			maxTokens: 8192,
			baseURL:   "http://localhost:11434/v1",
		},
		{
			name:      "Remote Ollama",
			model:     "llama3.1:8b",
			maxTokens: 16384,
			baseURL:   "https://ollama.example.com/v1",
		},
		{
			name:      "Custom port",
			model:     "mistral:7b",
			maxTokens: 4096,
			baseURL:   "http://localhost:9999/v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				client: nil,
				config: &base.Config{
					Enabled:   true,
					Model:     tt.model,
					MaxTokens: tt.maxTokens,
					BaseURL:   tt.baseURL,
				},
			}

			assert.Equal(t, tt.model, client.GetModel())
			assert.Equal(t, tt.maxTokens, client.GetMaxTokens())
			assert.Equal(t, tt.baseURL, client.GetBaseURL())
		})
	}
}

func TestOllamaModels(t *testing.T) {
	// Test various Ollama model configurations.
	models := []struct {
		modelID     string
		description string
	}{
		{"llama3.3:70b", "Llama 3.3 70B"},
		{"llama3.1:8b", "Llama 3.1 8B"},
		{"llama3.1:70b", "Llama 3.1 70B"},
		{"codellama:13b", "Code Llama 13B"},
		{"codellama:34b", "Code Llama 34B"},
		{"mistral:7b", "Mistral 7B"},
		{"mixtral:8x7b", "Mixtral 8x7B"},
		{"phi3:14b", "Phi-3 14B"},
		{"qwen2:7b", "Qwen 2 7B"},
		{"deepseek-coder:6.7b", "DeepSeek Coder 6.7B"},
	}

	for _, m := range models {
		t.Run(m.description, func(t *testing.T) {
			config := &base.Config{
				Enabled:   true,
				Model:     m.modelID,
				APIKeyEnv: DefaultAPIKeyEnv,
				MaxTokens: DefaultMaxTokens,
				BaseURL:   DefaultBaseURL,
			}

			client := &Client{
				client: nil,
				config: config,
			}

			assert.Equal(t, m.modelID, client.GetModel())
		})
	}
}

func TestOllamaBaseURLVariants(t *testing.T) {
	// Test various base URL configurations for Ollama.
	tests := []struct {
		name    string
		baseURL string
	}{
		{"Local default", "http://localhost:11434/v1"},
		{"Local with IP", "http://127.0.0.1:11434/v1"},
		{"Local with custom port", "http://localhost:9999/v1"},
		{"Docker network", "http://ollama:11434/v1"},
		{"Remote HTTPS", "https://ollama.example.com/v1"},
		{"Kubernetes service", "http://ollama-service.ai-namespace.svc.cluster.local:11434/v1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := base.ExtractConfig(&schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"ollama": {
								BaseURL: tt.baseURL,
							},
						},
					},
				},
			}, ProviderName, base.ProviderDefaults{
				Model:     DefaultModel,
				APIKeyEnv: DefaultAPIKeyEnv,
				MaxTokens: DefaultMaxTokens,
				BaseURL:   DefaultBaseURL,
			})

			assert.Equal(t, tt.baseURL, config.BaseURL)
		})
	}
}

func TestOllamaProviderName(t *testing.T) {
	// Verify the provider name constant.
	assert.Equal(t, "ollama", ProviderName)
}
