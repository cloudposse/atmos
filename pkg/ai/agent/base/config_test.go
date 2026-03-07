package base

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExtractConfig_DefaultConfiguration(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{},
		},
	}

	defaults := ProviderDefaults{
		Model:         "test-model",
		DefaultAPIKey: "test-key-value",
		MaxTokens:     4096,
	}

	config := ExtractConfig(atmosConfig, "test", defaults)

	assert.False(t, config.Enabled)
	assert.Equal(t, "test-model", config.Model)
	assert.Equal(t, "test-key-value", config.APIKey)
	assert.Equal(t, 4096, config.MaxTokens)
	assert.Empty(t, config.BaseURL)
}

func TestExtractConfig_EnabledConfiguration(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
			},
		},
	}

	defaults := ProviderDefaults{
		Model:         "default-model",
		DefaultAPIKey: "",
		MaxTokens:     1024,
	}

	config := ExtractConfig(atmosConfig, "test", defaults)

	assert.True(t, config.Enabled)
	assert.Equal(t, "default-model", config.Model)
}

func TestExtractConfig_ProviderSpecificOverrides(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"test": {
						Model:     "custom-model",
						ApiKey:    "custom-api-key-value",
						MaxTokens: 8192,
						BaseURL:   "https://custom.api.example.com",
					},
				},
			},
		},
	}

	defaults := ProviderDefaults{
		Model:         "default-model",
		DefaultAPIKey: "default-key",
		MaxTokens:     4096,
		BaseURL:       "",
	}

	config := ExtractConfig(atmosConfig, "test", defaults)

	assert.True(t, config.Enabled)
	assert.Equal(t, "custom-model", config.Model)
	assert.Equal(t, "custom-api-key-value", config.APIKey)
	assert.Equal(t, 8192, config.MaxTokens)
	assert.Equal(t, "https://custom.api.example.com", config.BaseURL)
}

func TestExtractConfig_PartialOverrides(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"test": {
						Model: "partial-model",
						// ApiKey, MaxTokens, and BaseURL not specified.
					},
				},
			},
		},
	}

	defaults := ProviderDefaults{
		Model:         "default-model",
		DefaultAPIKey: "default-key",
		MaxTokens:     4096,
		BaseURL:       "https://default.api.example.com",
	}

	config := ExtractConfig(atmosConfig, "test", defaults)

	assert.True(t, config.Enabled)
	assert.Equal(t, "partial-model", config.Model)
	assert.Equal(t, "default-key", config.APIKey)                      // Should use default.
	assert.Equal(t, 4096, config.MaxTokens)                            // Should use default.
	assert.Equal(t, "https://default.api.example.com", config.BaseURL) // Should use default.
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

	defaults := ProviderDefaults{
		Model:         "default-model",
		DefaultAPIKey: "",
		MaxTokens:     4096,
	}

	config := ExtractConfig(atmosConfig, "test", defaults)

	assert.True(t, config.Enabled)
	assert.Equal(t, "default-model", config.Model)
}

func TestExtractConfig_ProviderNotFound(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"other-provider": {
						Model: "other-model",
					},
				},
			},
		},
	}

	defaults := ProviderDefaults{
		Model:         "default-model",
		DefaultAPIKey: "default-key",
		MaxTokens:     4096,
	}

	config := ExtractConfig(atmosConfig, "test", defaults)

	// Should use defaults since "test" provider not found.
	assert.True(t, config.Enabled)
	assert.Equal(t, "default-model", config.Model)
	assert.Equal(t, "default-key", config.APIKey)
}

func TestExtractConfig_NilProviderConfig(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"test": nil,
				},
			},
		},
	}

	defaults := ProviderDefaults{
		Model:         "default-model",
		DefaultAPIKey: "default-key",
		MaxTokens:     4096,
	}

	config := ExtractConfig(atmosConfig, "test", defaults)

	// Should use defaults since provider config is nil.
	assert.True(t, config.Enabled)
	assert.Equal(t, "default-model", config.Model)
}

func TestExtractConfig_ZeroMaxTokens(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Providers: map[string]*schema.AIProviderConfig{
					"test": {
						Model:     "custom-model",
						MaxTokens: 0, // Explicitly set to 0.
					},
				},
			},
		},
	}

	defaults := ProviderDefaults{
		Model:         "default-model",
		DefaultAPIKey: "",
		MaxTokens:     4096,
	}

	config := ExtractConfig(atmosConfig, "test", defaults)

	// 0 is treated as "not set", so default should be used.
	assert.Equal(t, "custom-model", config.Model)
	assert.Equal(t, 4096, config.MaxTokens)
}

func TestProviderDefaults_Structure(t *testing.T) {
	defaults := ProviderDefaults{
		Model:         "test-model",
		DefaultAPIKey: "test-key",
		MaxTokens:     8192,
		BaseURL:       "https://api.example.com",
	}

	assert.Equal(t, "test-model", defaults.Model)
	assert.Equal(t, "test-key", defaults.DefaultAPIKey)
	assert.Equal(t, 8192, defaults.MaxTokens)
	assert.Equal(t, "https://api.example.com", defaults.BaseURL)
}

func TestConfig_Structure(t *testing.T) {
	config := Config{
		Enabled:   true,
		Model:     "gpt-4",
		APIKey:    "sk-test-key",
		MaxTokens: 4096,
		BaseURL:   "https://api.openai.com",
	}

	assert.True(t, config.Enabled)
	assert.Equal(t, "gpt-4", config.Model)
	assert.Equal(t, "sk-test-key", config.APIKey)
	assert.Equal(t, 4096, config.MaxTokens)
	assert.Equal(t, "https://api.openai.com", config.BaseURL)
}

func TestExtractConfig_TableDriven(t *testing.T) {
	tests := []struct {
		name           string
		atmosConfig    *schema.AtmosConfiguration
		providerName   string
		defaults       ProviderDefaults
		expectedConfig *Config
	}{
		{
			name: "anthropic defaults",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{},
				},
			},
			providerName: "anthropic",
			defaults: ProviderDefaults{
				Model:         "claude-sonnet-4-5-20250929",
				DefaultAPIKey: "",
				MaxTokens:     4096,
			},
			expectedConfig: &Config{
				Enabled:   false,
				Model:     "claude-sonnet-4-5-20250929",
				APIKey:    "",
				MaxTokens: 4096,
			},
		},
		{
			name: "openai defaults",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{},
				},
			},
			providerName: "openai",
			defaults: ProviderDefaults{
				Model:         "gpt-4o",
				DefaultAPIKey: "",
				MaxTokens:     4096,
			},
			expectedConfig: &Config{
				Enabled:   false,
				Model:     "gpt-4o",
				APIKey:    "",
				MaxTokens: 4096,
			},
		},
		{
			name: "gemini defaults",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{},
				},
			},
			providerName: "gemini",
			defaults: ProviderDefaults{
				Model:         "gemini-2.5-flash",
				DefaultAPIKey: "",
				MaxTokens:     8192,
			},
			expectedConfig: &Config{
				Enabled:   false,
				Model:     "gemini-2.5-flash",
				APIKey:    "",
				MaxTokens: 8192,
			},
		},
		{
			name: "grok with base URL",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"grok": {
								Model: "grok-4-latest",
							},
						},
					},
				},
			},
			providerName: "grok",
			defaults: ProviderDefaults{
				Model:         "grok-4-latest",
				DefaultAPIKey: "",
				MaxTokens:     4096,
				BaseURL:       "https://api.x.ai/v1",
			},
			expectedConfig: &Config{
				Enabled:   true,
				Model:     "grok-4-latest",
				APIKey:    "",
				MaxTokens: 4096,
				BaseURL:   "https://api.x.ai/v1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := ExtractConfig(tt.atmosConfig, tt.providerName, tt.defaults)
			assert.Equal(t, tt.expectedConfig, config)
		})
	}
}
