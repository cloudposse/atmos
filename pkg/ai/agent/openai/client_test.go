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

func TestNewClient_MissingAPIKey(t *testing.T) {
	// Use a unique env var name that definitely does not exist.
	envVar := "NONEXISTENT_OPENAI_KEY_XYZZY_12345"

	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"openai": {
						ApiKeyEnv: envVar,
					},
				},
			},
		},
	}

	client, err := NewClient(atmosConfig)
	assert.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "API key not found")
}

func TestDefaultConstants(t *testing.T) {
	assert.Equal(t, "openai", ProviderName)
	assert.Equal(t, 4096, DefaultMaxTokens)
	assert.Equal(t, "gpt-4o", DefaultModel)
	assert.Equal(t, "OPENAI_API_KEY", DefaultAPIKeyEnv)
}

func TestConfig_AllFields(t *testing.T) {
	config := &base.Config{
		Enabled:   true,
		Model:     "test-model",
		APIKeyEnv: "TEST_KEY",
		MaxTokens: 1000,
		BaseURL:   "https://api.example.com/v1",
	}

	assert.True(t, config.Enabled)
	assert.Equal(t, "test-model", config.Model)
	assert.Equal(t, "TEST_KEY", config.APIKeyEnv)
	assert.Equal(t, 1000, config.MaxTokens)
	assert.Equal(t, "https://api.example.com/v1", config.BaseURL)
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
	})

	// Should use defaults when providers is nil.
	assert.True(t, config.Enabled)
	assert.Equal(t, DefaultModel, config.Model)
	assert.Equal(t, DefaultAPIKeyEnv, config.APIKeyEnv)
	assert.Equal(t, DefaultMaxTokens, config.MaxTokens)
}

func TestExtractConfig_DifferentProviderOnly(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
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
	}

	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:     DefaultModel,
		APIKeyEnv: DefaultAPIKeyEnv,
		MaxTokens: DefaultMaxTokens,
	})

	// Should use defaults when this provider is not configured.
	assert.True(t, config.Enabled)
	assert.Equal(t, DefaultModel, config.Model)
	assert.Equal(t, DefaultAPIKeyEnv, config.APIKeyEnv)
	assert.Equal(t, DefaultMaxTokens, config.MaxTokens)
}

func TestExtractConfig_NilProviderConfig(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"openai": nil, // Explicitly nil provider config.
				},
			},
		},
	}

	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:     DefaultModel,
		APIKeyEnv: DefaultAPIKeyEnv,
		MaxTokens: DefaultMaxTokens,
	})

	// Should use defaults when provider config is nil.
	assert.True(t, config.Enabled)
	assert.Equal(t, DefaultModel, config.Model)
	assert.Equal(t, DefaultAPIKeyEnv, config.APIKeyEnv)
	assert.Equal(t, DefaultMaxTokens, config.MaxTokens)
}

func TestClientGetters_CustomValues(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		maxTokens int
	}{
		{
			name:      "Default GPT-4o",
			model:     "gpt-4o",
			maxTokens: 4096,
		},
		{
			name:      "GPT-4 Turbo",
			model:     "gpt-4-turbo",
			maxTokens: 8192,
		},
		{
			name:      "GPT-4o Mini",
			model:     "gpt-4o-mini",
			maxTokens: 16384,
		},
		{
			name:      "GPT-3.5 Turbo",
			model:     "gpt-3.5-turbo",
			maxTokens: 4096,
		},
		{
			name:      "GPT-5 Future",
			model:     "gpt-5",
			maxTokens: 32768,
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
				},
			}

			assert.Equal(t, tt.model, client.GetModel())
			assert.Equal(t, tt.maxTokens, client.GetMaxTokens())
		})
	}
}

func TestOpenAIModels(t *testing.T) {
	// Test various OpenAI model configurations.
	models := []struct {
		modelID     string
		description string
	}{
		{"gpt-4o", "GPT-4o"},
		{"gpt-4o-mini", "GPT-4o Mini"},
		{"gpt-4-turbo", "GPT-4 Turbo"},
		{"gpt-4-turbo-preview", "GPT-4 Turbo Preview"},
		{"gpt-3.5-turbo", "GPT-3.5 Turbo"},
		{"gpt-3.5-turbo-16k", "GPT-3.5 Turbo 16K"},
		{"o1-preview", "O1 Preview"},
		{"o1-mini", "O1 Mini"},
		{"chatgpt-4o-latest", "ChatGPT-4o Latest"},
	}

	for _, m := range models {
		t.Run(m.description, func(t *testing.T) {
			config := &base.Config{
				Enabled:   true,
				Model:     m.modelID,
				APIKeyEnv: DefaultAPIKeyEnv,
				MaxTokens: DefaultMaxTokens,
			}

			client := &Client{
				client: nil,
				config: config,
			}

			assert.Equal(t, m.modelID, client.GetModel())
		})
	}
}

func TestExtractConfig_CustomBaseURL(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"openai": {
						BaseURL: "https://custom.openai.azure.com/v1",
					},
				},
			},
		},
	}

	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:     DefaultModel,
		APIKeyEnv: DefaultAPIKeyEnv,
		MaxTokens: DefaultMaxTokens,
	})

	assert.Equal(t, "https://custom.openai.azure.com/v1", config.BaseURL)
}

func TestExtractConfig_AllOverrides(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"openai": {
						Model:     "o1-preview",
						ApiKeyEnv: "CUSTOM_OPENAI_KEY",
						MaxTokens: 32768,
						BaseURL:   "https://api.custom.openai.com/v2",
					},
				},
			},
		},
	}

	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:     DefaultModel,
		APIKeyEnv: DefaultAPIKeyEnv,
		MaxTokens: DefaultMaxTokens,
	})

	assert.True(t, config.Enabled)
	assert.Equal(t, "o1-preview", config.Model)
	assert.Equal(t, "CUSTOM_OPENAI_KEY", config.APIKeyEnv)
	assert.Equal(t, 32768, config.MaxTokens)
	assert.Equal(t, "https://api.custom.openai.com/v2", config.BaseURL)
}

func TestExtractConfig_EmptyStringValues(t *testing.T) {
	// Test that empty strings use defaults.
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"openai": {
						Model:     "", // Empty should use default.
						ApiKeyEnv: "", // Empty should use default.
						BaseURL:   "", // Empty should use default.
					},
				},
			},
		},
	}

	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:     DefaultModel,
		APIKeyEnv: DefaultAPIKeyEnv,
		MaxTokens: DefaultMaxTokens,
	})

	assert.Equal(t, DefaultModel, config.Model)
	assert.Equal(t, DefaultAPIKeyEnv, config.APIKeyEnv)
	assert.Empty(t, config.BaseURL) // BaseURL empty is valid for OpenAI (uses SDK default).
}

func TestNewClient_WithValidAPIKey(t *testing.T) {
	// This test verifies that NewClient correctly creates a client when API key is present.
	// We set a temporary env var for this test.
	envVar := "TEST_OPENAI_KEY_FOR_CLIENT_CREATION"
	t.Setenv(envVar, "test-api-key-value")

	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"openai": {
						ApiKeyEnv: envVar,
					},
				},
			},
		},
	}

	client, err := NewClient(atmosConfig)
	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.NotNil(t, client.client)
	assert.NotNil(t, client.config)
	assert.Equal(t, DefaultModel, client.GetModel())
	assert.Equal(t, DefaultMaxTokens, client.GetMaxTokens())
}

func TestClient_StructFields(t *testing.T) {
	// Test that Client struct properly stores config.
	config := &base.Config{
		Enabled:   true,
		Model:     "gpt-4o-mini",
		APIKeyEnv: "TEST_KEY",
		MaxTokens: 2048,
		BaseURL:   "https://custom.openai.com/v1",
	}

	client := &Client{
		client: nil,
		config: config,
	}

	assert.Equal(t, "gpt-4o-mini", client.GetModel())
	assert.Equal(t, 2048, client.GetMaxTokens())
}

func TestOpenAIModels_EdgeCases(t *testing.T) {
	// Test various model edge cases.
	models := []struct {
		modelID     string
		description string
	}{
		{"gpt-4o-2024-11-20", "GPT-4o with specific date"},
		{"gpt-4-turbo-2024-04-09", "GPT-4 Turbo with date"},
		{"gpt-3.5-turbo-0125", "GPT-3.5 Turbo with version"},
		{"custom-model", "Custom model name"},
		{"gpt-4o-realtime-preview", "GPT-4o Realtime Preview"},
	}

	for _, m := range models {
		t.Run(m.description, func(t *testing.T) {
			config := &base.Config{
				Enabled:   true,
				Model:     m.modelID,
				APIKeyEnv: DefaultAPIKeyEnv,
				MaxTokens: DefaultMaxTokens,
			}

			client := &Client{
				client: nil,
				config: config,
			}

			assert.Equal(t, m.modelID, client.GetModel())
		})
	}
}

func TestExtractConfig_MaxTokensEdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		maxTokens      int
		expectedTokens int
	}{
		{
			name:           "Zero max tokens uses default",
			maxTokens:      0,
			expectedTokens: DefaultMaxTokens,
		},
		{
			name:           "Negative max tokens uses default",
			maxTokens:      -1,
			expectedTokens: DefaultMaxTokens,
		},
		{
			name:           "Positive max tokens used",
			maxTokens:      8192,
			expectedTokens: 8192,
		},
		{
			name:           "Very large max tokens",
			maxTokens:      128000,
			expectedTokens: 128000,
		},
		{
			name:           "Small max tokens",
			maxTokens:      100,
			expectedTokens: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"openai": {
								MaxTokens: tt.maxTokens,
							},
						},
					},
				},
			}

			config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
				Model:     DefaultModel,
				APIKeyEnv: DefaultAPIKeyEnv,
				MaxTokens: DefaultMaxTokens,
			})

			assert.Equal(t, tt.expectedTokens, config.MaxTokens)
		})
	}
}

func TestNewClient_ErrorPaths(t *testing.T) {
	tests := []struct {
		name          string
		atmosConfig   *schema.AtmosConfiguration
		expectedError string
	}{
		{
			name: "AI disabled",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: false,
					},
				},
			},
			expectedError: "AI features are disabled",
		},
		{
			name: "Missing API key",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"openai": {
								ApiKeyEnv: "NONEXISTENT_OPENAI_KEY_12345_TEST",
							},
						},
					},
				},
			},
			expectedError: "API key not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.atmosConfig)
			assert.Error(t, err)
			assert.Nil(t, client)
			assert.Contains(t, err.Error(), tt.expectedError)
		})
	}
}

func TestGetters_VariousConfigurations(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		maxTokens int
	}{
		{"GPT-4o with 4096", "gpt-4o", 4096},
		{"GPT-4o-mini with 16384", "gpt-4o-mini", 16384},
		{"GPT-4 Turbo with 128000", "gpt-4-turbo", 128000},
		{"GPT-3.5 Turbo with 4096", "gpt-3.5-turbo", 4096},
		{"O1 Preview with 32768", "o1-preview", 32768},
		{"O1 Mini with 65536", "o1-mini", 65536},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				config: &base.Config{
					Model:     tt.model,
					MaxTokens: tt.maxTokens,
				},
			}

			assert.Equal(t, tt.model, client.GetModel())
			assert.Equal(t, tt.maxTokens, client.GetMaxTokens())
		})
	}
}

func TestProviderName_Value(t *testing.T) {
	// Verify provider name constant.
	assert.Equal(t, "openai", ProviderName)
	assert.NotEmpty(t, ProviderName)
}

func TestDefaultValues_Validation(t *testing.T) {
	// Validate default constants.
	assert.Greater(t, DefaultMaxTokens, 0, "DefaultMaxTokens should be positive")
	assert.LessOrEqual(t, DefaultMaxTokens, 200000, "DefaultMaxTokens should be reasonable")
	assert.NotEmpty(t, DefaultModel, "DefaultModel should not be empty")
	assert.Contains(t, DefaultModel, "gpt", "DefaultModel should be a GPT model")
	assert.NotEmpty(t, DefaultAPIKeyEnv, "DefaultAPIKeyEnv should not be empty")
	assert.Contains(t, DefaultAPIKeyEnv, "OPENAI", "DefaultAPIKeyEnv should reference OpenAI")
	assert.Contains(t, DefaultAPIKeyEnv, "API_KEY", "DefaultAPIKeyEnv should mention API_KEY")
}

func TestExtractConfig_BaseURLCustomization(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		expectedURL string
	}{
		{
			name:        "Empty base URL (uses SDK default)",
			baseURL:     "",
			expectedURL: "",
		},
		{
			name:        "Azure OpenAI endpoint",
			baseURL:     "https://myresource.openai.azure.com/",
			expectedURL: "https://myresource.openai.azure.com/",
		},
		{
			name:        "Custom proxy endpoint",
			baseURL:     "https://proxy.example.com/openai/v1",
			expectedURL: "https://proxy.example.com/openai/v1",
		},
		{
			name:        "Local development endpoint",
			baseURL:     "http://localhost:8080/v1",
			expectedURL: "http://localhost:8080/v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"openai": {
								BaseURL: tt.baseURL,
							},
						},
					},
				},
			}

			config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
				Model:     DefaultModel,
				APIKeyEnv: DefaultAPIKeyEnv,
				MaxTokens: DefaultMaxTokens,
			})

			assert.Equal(t, tt.expectedURL, config.BaseURL)
		})
	}
}

//nolint:dupl // Similar test setup to other provider tests.
func TestExtractConfig_CompleteOverride(t *testing.T) {
	// Test overriding all configuration values.
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"openai": {
						Model:     "gpt-4-turbo-preview",
						ApiKeyEnv: "CUSTOM_OPENAI_TOKEN",
						MaxTokens: 65536,
						BaseURL:   "https://custom-api.openai.com/v2",
					},
				},
			},
		},
	}

	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:     DefaultModel,
		APIKeyEnv: DefaultAPIKeyEnv,
		MaxTokens: DefaultMaxTokens,
	})

	assert.True(t, config.Enabled)
	assert.Equal(t, "gpt-4-turbo-preview", config.Model)
	assert.Equal(t, "CUSTOM_OPENAI_TOKEN", config.APIKeyEnv)
	assert.Equal(t, 65536, config.MaxTokens)
	assert.Equal(t, "https://custom-api.openai.com/v2", config.BaseURL)
}

func TestNewClient_NilAtmosConfig(t *testing.T) {
	// Test that NewClient handles nil atmosConfig.
	// This should panic in base.ExtractConfig, as it's a programming error.
	assert.Panics(t, func() {
		_, _ = NewClient(nil)
	})
}

func TestOpenAIModels_O1Series(t *testing.T) {
	// Test O1 series models specifically (they have different token handling).
	models := []string{
		"o1-preview",
		"o1-preview-2024-09-12",
		"o1-mini",
		"o1-mini-2024-09-12",
	}

	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			client := &Client{
				config: &base.Config{
					Model:     model,
					MaxTokens: DefaultMaxTokens,
				},
			}

			assert.Equal(t, model, client.GetModel())
		})
	}
}

func TestConfig_BooleanFields(t *testing.T) {
	// Test that Enabled field works correctly.
	tests := []struct {
		name    string
		enabled bool
	}{
		{"Enabled true", true},
		{"Enabled false", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &base.Config{
				Enabled:   tt.enabled,
				Model:     DefaultModel,
				APIKeyEnv: DefaultAPIKeyEnv,
				MaxTokens: DefaultMaxTokens,
			}

			assert.Equal(t, tt.enabled, config.Enabled)
		})
	}
}

func TestExtractConfig_MultipleProviders(t *testing.T) {
	// Test that OpenAI config is extracted correctly when multiple providers are configured.
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"anthropic": {
						Model: "claude-sonnet-4-20250514",
					},
					"openai": {
						Model:     "gpt-4o",
						MaxTokens: 8192,
					},
					"ollama": {
						Model: "llama3.3:70b",
					},
				},
			},
		},
	}

	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:     DefaultModel,
		APIKeyEnv: DefaultAPIKeyEnv,
		MaxTokens: DefaultMaxTokens,
	})

	// Should extract only OpenAI config.
	assert.Equal(t, "gpt-4o", config.Model)
	assert.Equal(t, 8192, config.MaxTokens)
	assert.Equal(t, DefaultAPIKeyEnv, config.APIKeyEnv)
}

func TestConstants_Consistency(t *testing.T) {
	// Test that constants are consistent with each other.
	assert.Equal(t, "openai", ProviderName)
	assert.Equal(t, "gpt-4o", DefaultModel)
	assert.Equal(t, 4096, DefaultMaxTokens)
	assert.Equal(t, "OPENAI_API_KEY", DefaultAPIKeyEnv)

	// Verify no unexpected changes.
	assert.NotEqual(t, "", ProviderName)
	assert.NotEqual(t, "", DefaultModel)
	assert.NotEqual(t, 0, DefaultMaxTokens)
	assert.NotEqual(t, "", DefaultAPIKeyEnv)
}
