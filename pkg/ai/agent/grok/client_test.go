//nolint:dupl // Test files contain similar setup code by design for isolation and clarity.
package grok

import (
	"strings"
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
				Model:     "grok-4-latest",
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
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"grok": {
								Model:     "grok-4",
								ApiKeyEnv: "CUSTOM_XAI_KEY",
								MaxTokens: 8192,
								BaseURL:   "https://custom.api.x.ai/v1",
							},
						},
					},
				},
			},
			expectedConfig: &base.Config{
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
						Providers: map[string]*schema.AIProviderConfig{
							"grok": {
								Model: "grok-2",
							},
						},
					},
				},
			},
			expectedConfig: &base.Config{
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
		Model:     "grok-4-latest",
		APIKeyEnv: "XAI_API_KEY",
		MaxTokens: 4096,
		BaseURL:   "https://api.x.ai/v1",
	}

	client := &Client{
		client: nil, // We don't need a real client for testing getters.
		config: config,
	}

	assert.Equal(t, "grok-4-latest", client.GetModel())
	assert.Equal(t, 4096, client.GetMaxTokens())
	assert.Equal(t, "https://api.x.ai/v1", client.GetBaseURL())
}

func TestNewClient_MissingAPIKey(t *testing.T) {
	// Use a unique env var name that definitely does not exist.
	envVar := "NONEXISTENT_GROK_KEY_XYZZY_12345"

	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"grok": {
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
	assert.Equal(t, "grok", ProviderName)
	assert.Equal(t, 4096, DefaultMaxTokens)
	assert.Equal(t, "grok-4-latest", DefaultModel)
	assert.Equal(t, "XAI_API_KEY", DefaultAPIKeyEnv)
	assert.Equal(t, "https://api.x.ai/v1", DefaultBaseURL)
}

func TestConfig_AllFields(t *testing.T) {
	config := &base.Config{
		Enabled:   true,
		Model:     "test-model",
		APIKeyEnv: "TEST_KEY",
		MaxTokens: 1000,
		BaseURL:   "https://test.example.com/v1",
	}

	assert.True(t, config.Enabled)
	assert.Equal(t, "test-model", config.Model)
	assert.Equal(t, "TEST_KEY", config.APIKeyEnv)
	assert.Equal(t, 1000, config.MaxTokens)
	assert.Equal(t, "https://test.example.com/v1", config.BaseURL)
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
					"grok": nil, // Explicitly nil provider config.
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
			name:      "Default values",
			model:     "grok-4-latest",
			maxTokens: 4096,
			baseURL:   "https://api.x.ai/v1",
		},
		{
			name:      "Custom model",
			model:     "grok-2-vision-1212",
			maxTokens: 8192,
			baseURL:   "https://api.x.ai/v1",
		},
		{
			name:      "High token limit",
			model:     "grok-4-latest",
			maxTokens: 131072,
			baseURL:   "https://api.x.ai/v1",
		},
		{
			name:      "Custom base URL",
			model:     "grok-4-latest",
			maxTokens: 4096,
			baseURL:   "https://custom.xai.example.com/api",
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

func TestGrokModels(t *testing.T) {
	// Test various Grok model configurations.
	models := []struct {
		modelID     string
		description string
	}{
		{"grok-4-latest", "Grok 4 Latest"},
		{"grok-4", "Grok 4"},
		{"grok-2-vision-1212", "Grok 2 Vision"},
		{"grok-2-1212", "Grok 2"},
		{"grok-beta", "Grok Beta"},
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

func TestExtractConfig_CustomBaseURL(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"grok": {
						BaseURL: "https://custom.xai.example.com/v1",
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

	assert.Equal(t, "https://custom.xai.example.com/v1", config.BaseURL)
}

func TestExtractConfig_AllOverrides(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"grok": {
						Model:     "grok-2-vision-1212",
						ApiKeyEnv: "CUSTOM_GROK_KEY",
						MaxTokens: 32768,
						BaseURL:   "https://api.custom.xai.com/v2",
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

	assert.True(t, config.Enabled)
	assert.Equal(t, "grok-2-vision-1212", config.Model)
	assert.Equal(t, "CUSTOM_GROK_KEY", config.APIKeyEnv)
	assert.Equal(t, 32768, config.MaxTokens)
	assert.Equal(t, "https://api.custom.xai.com/v2", config.BaseURL)
}

func TestNewClient_WithValidAPIKey(t *testing.T) {
	// Set a valid API key for testing.
	envVar := "TEST_GROK_API_KEY_12345"
	t.Setenv(envVar, "test-api-key-value")

	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"grok": {
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
	assert.Equal(t, DefaultBaseURL, client.GetBaseURL())
}

func TestNewClient_CustomConfiguration(t *testing.T) {
	envVar := "TEST_GROK_CUSTOM_KEY_99999"
	t.Setenv(envVar, "custom-key")

	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"grok": {
						Model:     "grok-2-1212",
						ApiKeyEnv: envVar,
						MaxTokens: 8192,
						BaseURL:   "https://custom.api.x.ai/v1",
					},
				},
			},
		},
	}

	client, err := NewClient(atmosConfig)
	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, "grok-2-1212", client.GetModel())
	assert.Equal(t, 8192, client.GetMaxTokens())
	assert.Equal(t, "https://custom.api.x.ai/v1", client.GetBaseURL())
}

func TestClient_StructFields(t *testing.T) {
	config := &base.Config{
		Enabled:   true,
		Model:     "test-model",
		APIKeyEnv: "TEST_KEY",
		MaxTokens: 2000,
		BaseURL:   "https://test.api.com",
	}

	client := &Client{
		client: nil,
		config: config,
	}

	// Test getters.
	assert.Equal(t, "test-model", client.GetModel())
	assert.Equal(t, 2000, client.GetMaxTokens())
	assert.Equal(t, "https://test.api.com", client.GetBaseURL())
}

func TestExtractConfig_EdgeCases(t *testing.T) {
	// Test with MaxTokens = 0 (should use default).
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"grok": {
						MaxTokens: 0, // Zero should not override default.
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

	// MaxTokens should use default since 0 is not > 0.
	assert.Equal(t, DefaultMaxTokens, config.MaxTokens)
}

func TestExtractConfig_PartialOverrides(t *testing.T) {
	tests := []struct {
		name           string
		providerConfig *schema.AIProviderConfig
		expectedModel  string
		expectedEnv    string
		expectedTokens int
		expectedURL    string
	}{
		{
			name: "Only model override",
			providerConfig: &schema.AIProviderConfig{
				Model: "grok-2-vision-1212",
			},
			expectedModel:  "grok-2-vision-1212",
			expectedEnv:    DefaultAPIKeyEnv,
			expectedTokens: DefaultMaxTokens,
			expectedURL:    DefaultBaseURL,
		},
		{
			name: "Only API key env override",
			providerConfig: &schema.AIProviderConfig{
				ApiKeyEnv: "CUSTOM_KEY",
			},
			expectedModel:  DefaultModel,
			expectedEnv:    "CUSTOM_KEY",
			expectedTokens: DefaultMaxTokens,
			expectedURL:    DefaultBaseURL,
		},
		{
			name: "Only max tokens override",
			providerConfig: &schema.AIProviderConfig{
				MaxTokens: 10000,
			},
			expectedModel:  DefaultModel,
			expectedEnv:    DefaultAPIKeyEnv,
			expectedTokens: 10000,
			expectedURL:    DefaultBaseURL,
		},
		{
			name: "Only base URL override",
			providerConfig: &schema.AIProviderConfig{
				BaseURL: "https://proxy.example.com/xai",
			},
			expectedModel:  DefaultModel,
			expectedEnv:    DefaultAPIKeyEnv,
			expectedTokens: DefaultMaxTokens,
			expectedURL:    "https://proxy.example.com/xai",
		},
		{
			name: "Model and tokens override",
			providerConfig: &schema.AIProviderConfig{
				Model:     "grok-4",
				MaxTokens: 16000,
			},
			expectedModel:  "grok-4",
			expectedEnv:    DefaultAPIKeyEnv,
			expectedTokens: 16000,
			expectedURL:    DefaultBaseURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"grok": tt.providerConfig,
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

			assert.Equal(t, tt.expectedModel, config.Model)
			assert.Equal(t, tt.expectedEnv, config.APIKeyEnv)
			assert.Equal(t, tt.expectedTokens, config.MaxTokens)
			assert.Equal(t, tt.expectedURL, config.BaseURL)
		})
	}
}

func TestProviderName_Constant(t *testing.T) {
	assert.Equal(t, "grok", ProviderName)
	// Verify it's a lowercase string suitable for config lookups.
	assert.Equal(t, ProviderName, "grok")
	assert.NotContains(t, ProviderName, " ")
	assert.NotContains(t, ProviderName, "-")
}

func TestDefaultValues_AllConstants(t *testing.T) {
	// DefaultMaxTokens should be reasonable for most use cases.
	assert.Greater(t, DefaultMaxTokens, 0)
	assert.LessOrEqual(t, DefaultMaxTokens, 200000)

	// DefaultModel should be a valid model string.
	assert.NotEmpty(t, DefaultModel)
	assert.Contains(t, DefaultModel, "grok")

	// DefaultAPIKeyEnv should follow standard naming conventions.
	assert.NotEmpty(t, DefaultAPIKeyEnv)
	assert.Contains(t, DefaultAPIKeyEnv, "XAI")
	assert.Contains(t, DefaultAPIKeyEnv, "API_KEY")

	// DefaultBaseURL should be xAI's official endpoint.
	assert.NotEmpty(t, DefaultBaseURL)
	assert.Contains(t, DefaultBaseURL, "api.x.ai")
	assert.True(t, strings.HasPrefix(DefaultBaseURL, "https://"), "BaseURL should start with https://")
}

func TestConfig_BaseURLField(t *testing.T) {
	config := &base.Config{
		Enabled:   true,
		Model:     "test-model",
		APIKeyEnv: "TEST_KEY",
		MaxTokens: 1000,
		BaseURL:   "https://test.example.com/v1",
	}

	assert.True(t, config.Enabled)
	assert.Equal(t, "test-model", config.Model)
	assert.Equal(t, "TEST_KEY", config.APIKeyEnv)
	assert.Equal(t, 1000, config.MaxTokens)
	assert.Equal(t, "https://test.example.com/v1", config.BaseURL)
}

func TestClientGetters_EdgeValues(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		maxTokens int
		baseURL   string
	}{
		{
			name:      "Minimum tokens",
			model:     "test-model",
			maxTokens: 1,
			baseURL:   "https://api.x.ai/v1",
		},
		{
			name:      "Maximum tokens",
			model:     "test-model",
			maxTokens: 200000,
			baseURL:   "https://api.x.ai/v1",
		},
		{
			name:      "Empty base URL",
			model:     "grok-4-latest",
			maxTokens: 4096,
			baseURL:   "",
		},
		{
			name:      "Custom protocol base URL",
			model:     "grok-4-latest",
			maxTokens: 4096,
			baseURL:   "http://localhost:8080/v1",
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

func TestExtractConfig_EmptyModel(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"grok": {
						Model: "", // Empty model should use default.
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

	// Empty model should use default.
	assert.Equal(t, DefaultModel, config.Model)
}

func TestExtractConfig_EmptyAPIKeyEnv(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"grok": {
						ApiKeyEnv: "", // Empty API key env should use default.
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

	// Empty API key env should use default.
	assert.Equal(t, DefaultAPIKeyEnv, config.APIKeyEnv)
}

func TestExtractConfig_EmptyBaseURL(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"grok": {
						BaseURL: "", // Empty base URL should use default.
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

	// Empty base URL should use default.
	assert.Equal(t, DefaultBaseURL, config.BaseURL)
}

func TestNewClient_NilAtmosConfig(t *testing.T) {
	// Test with nil AtmosConfiguration - should not panic.
	// The ExtractConfig function handles nil safely.
	atmosConfig := &schema.AtmosConfiguration{}

	client, err := NewClient(atmosConfig)
	assert.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "AI features are disabled")
}

func TestNewClient_AIDisabledInAtmosConfig(t *testing.T) {
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

func TestExtractConfig_NegativeMaxTokens(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"grok": {
						MaxTokens: -100, // Negative should use default.
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

	// Negative MaxTokens should use default since -100 is not > 0.
	assert.Equal(t, DefaultMaxTokens, config.MaxTokens)
}

func TestGrokModels_VariousVersions(t *testing.T) {
	// Test various Grok model version formats.
	models := []string{
		"grok-4-latest",
		"grok-4",
		"grok-3-vision-1212",
		"grok-2-vision-1212",
		"grok-2-1212",
		"grok-2",
		"grok-beta",
		"grok-preview",
		"grok-experimental",
	}

	for _, modelID := range models {
		t.Run(modelID, func(t *testing.T) {
			client := &Client{
				client: nil,
				config: &base.Config{
					Enabled:   true,
					Model:     modelID,
					APIKeyEnv: DefaultAPIKeyEnv,
					MaxTokens: DefaultMaxTokens,
					BaseURL:   DefaultBaseURL,
				},
			}

			assert.Equal(t, modelID, client.GetModel())
		})
	}
}

func TestClient_ConfigVariations(t *testing.T) {
	tests := []struct {
		name             string
		model            string
		maxTokens        int
		baseURL          string
		expectedModel    string
		expectedMaxToken int
		expectedBaseURL  string
	}{
		{
			name:             "Default configuration",
			model:            DefaultModel,
			maxTokens:        DefaultMaxTokens,
			baseURL:          DefaultBaseURL,
			expectedModel:    "grok-4-latest",
			expectedMaxToken: 4096,
			expectedBaseURL:  "https://api.x.ai/v1",
		},
		{
			name:             "Grok 2 Vision",
			model:            "grok-2-vision-1212",
			maxTokens:        8192,
			baseURL:          DefaultBaseURL,
			expectedModel:    "grok-2-vision-1212",
			expectedMaxToken: 8192,
			expectedBaseURL:  "https://api.x.ai/v1",
		},
		{
			name:             "High token limit",
			model:            "grok-4-latest",
			maxTokens:        131072,
			baseURL:          DefaultBaseURL,
			expectedModel:    "grok-4-latest",
			expectedMaxToken: 131072,
			expectedBaseURL:  "https://api.x.ai/v1",
		},
		{
			name:             "Custom proxy",
			model:            "grok-4-latest",
			maxTokens:        4096,
			baseURL:          "https://proxy.company.com/xai/v1",
			expectedModel:    "grok-4-latest",
			expectedMaxToken: 4096,
			expectedBaseURL:  "https://proxy.company.com/xai/v1",
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

			assert.Equal(t, tt.expectedModel, client.GetModel())
			assert.Equal(t, tt.expectedMaxToken, client.GetMaxTokens())
			assert.Equal(t, tt.expectedBaseURL, client.GetBaseURL())
		})
	}
}

func TestNewClient_MultipleAPIKeyFormats(t *testing.T) {
	tests := []struct {
		name   string
		envVar string
		value  string
	}{
		{
			name:   "Standard format",
			envVar: "TEST_GROK_KEY_1",
			value:  "xai-abc123def456",
		},
		{
			name:   "Long key",
			envVar: "TEST_GROK_KEY_2",
			value:  "xai-" + strings.Repeat("abcdefghij", 10), // 100 character key.
		},
		{
			name:   "Short key",
			envVar: "TEST_GROK_KEY_3",
			value:  "key",
		},
		{
			name:   "Key with special chars",
			envVar: "TEST_GROK_KEY_4",
			value:  "xai_key-123.test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.envVar, tt.value)

			atmosConfig := &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"grok": {
								ApiKeyEnv: tt.envVar,
							},
						},
					},
				},
			}

			client, err := NewClient(atmosConfig)
			assert.NoError(t, err)
			assert.NotNil(t, client)
		})
	}
}

func TestExtractConfig_MultipleProviders(t *testing.T) {
	// Test that Grok config is extracted correctly when multiple providers are configured.
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"openai": {
						Model: "gpt-4o",
					},
					"grok": {
						Model:     "grok-2-vision-1212",
						MaxTokens: 8192,
					},
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
		BaseURL:   DefaultBaseURL,
	})

	// Should extract only Grok config, not others.
	assert.True(t, config.Enabled)
	assert.Equal(t, "grok-2-vision-1212", config.Model)
	assert.Equal(t, 8192, config.MaxTokens)
	assert.Equal(t, DefaultAPIKeyEnv, config.APIKeyEnv)
	assert.Equal(t, DefaultBaseURL, config.BaseURL)
}
