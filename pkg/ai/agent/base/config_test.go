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
		Model:     "test-model",
		APIKeyEnv: "TEST_API_KEY",
		MaxTokens: 4096,
	}

	config := ExtractConfig(atmosConfig, "test", defaults)

	assert.False(t, config.Enabled)
	assert.Equal(t, "test-model", config.Model)
	assert.Equal(t, "TEST_API_KEY", config.APIKeyEnv)
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
		Model:     "default-model",
		APIKeyEnv: "DEFAULT_API_KEY",
		MaxTokens: 1024,
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
						ApiKeyEnv: "CUSTOM_API_KEY",
						MaxTokens: 8192,
						BaseURL:   "https://custom.api.example.com",
					},
				},
			},
		},
	}

	defaults := ProviderDefaults{
		Model:     "default-model",
		APIKeyEnv: "DEFAULT_API_KEY",
		MaxTokens: 4096,
		BaseURL:   "",
	}

	config := ExtractConfig(atmosConfig, "test", defaults)

	assert.True(t, config.Enabled)
	assert.Equal(t, "custom-model", config.Model)
	assert.Equal(t, "CUSTOM_API_KEY", config.APIKeyEnv)
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
						// ApiKeyEnv, MaxTokens, and BaseURL not specified
					},
				},
			},
		},
	}

	defaults := ProviderDefaults{
		Model:     "default-model",
		APIKeyEnv: "DEFAULT_API_KEY",
		MaxTokens: 4096,
		BaseURL:   "https://default.api.example.com",
	}

	config := ExtractConfig(atmosConfig, "test", defaults)

	assert.True(t, config.Enabled)
	assert.Equal(t, "partial-model", config.Model)
	assert.Equal(t, "DEFAULT_API_KEY", config.APIKeyEnv)               // Should use default
	assert.Equal(t, 4096, config.MaxTokens)                            // Should use default
	assert.Equal(t, "https://default.api.example.com", config.BaseURL) // Should use default
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
		Model:     "default-model",
		APIKeyEnv: "DEFAULT_API_KEY",
		MaxTokens: 4096,
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
		Model:     "default-model",
		APIKeyEnv: "DEFAULT_API_KEY",
		MaxTokens: 4096,
	}

	config := ExtractConfig(atmosConfig, "test", defaults)

	// Should use defaults since "test" provider not found.
	assert.True(t, config.Enabled)
	assert.Equal(t, "default-model", config.Model)
	assert.Equal(t, "DEFAULT_API_KEY", config.APIKeyEnv)
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
		Model:     "default-model",
		APIKeyEnv: "DEFAULT_API_KEY",
		MaxTokens: 4096,
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
						MaxTokens: 0, // Explicitly set to 0
					},
				},
			},
		},
	}

	defaults := ProviderDefaults{
		Model:     "default-model",
		APIKeyEnv: "DEFAULT_API_KEY",
		MaxTokens: 4096,
	}

	config := ExtractConfig(atmosConfig, "test", defaults)

	// 0 is treated as "not set", so default should be used.
	assert.Equal(t, "custom-model", config.Model)
	assert.Equal(t, 4096, config.MaxTokens)
}

func TestGetAPIKey_FromEnvironment(t *testing.T) {
	envVar := "TEST_AI_API_KEY_12345"
	expectedValue := "sk-test-key-value"

	// Set the environment variable using t.Setenv for automatic cleanup.
	t.Setenv(envVar, expectedValue)

	result := GetAPIKey(envVar)
	assert.Equal(t, expectedValue, result)
}

func TestGetAPIKey_NotSet(t *testing.T) {
	// Use a unique variable name that won't exist in the environment.
	envVar := "NONEXISTENT_API_KEY_XYZZY_TEST_" + t.Name()

	result := GetAPIKey(envVar)
	assert.Empty(t, result)
}

func TestGetAPIKey_EmptyValue(t *testing.T) {
	envVar := "EMPTY_API_KEY_TEST"

	// Set to empty string using t.Setenv for automatic cleanup.
	t.Setenv(envVar, "")

	result := GetAPIKey(envVar)
	assert.Empty(t, result)
}

func TestProviderDefaults_Structure(t *testing.T) {
	defaults := ProviderDefaults{
		Model:     "test-model",
		APIKeyEnv: "TEST_KEY",
		MaxTokens: 8192,
		BaseURL:   "https://api.example.com",
	}

	assert.Equal(t, "test-model", defaults.Model)
	assert.Equal(t, "TEST_KEY", defaults.APIKeyEnv)
	assert.Equal(t, 8192, defaults.MaxTokens)
	assert.Equal(t, "https://api.example.com", defaults.BaseURL)
}

func TestConfig_Structure(t *testing.T) {
	config := Config{
		Enabled:   true,
		Model:     "gpt-4",
		APIKeyEnv: "OPENAI_API_KEY",
		MaxTokens: 4096,
		BaseURL:   "https://api.openai.com",
	}

	assert.True(t, config.Enabled)
	assert.Equal(t, "gpt-4", config.Model)
	assert.Equal(t, "OPENAI_API_KEY", config.APIKeyEnv)
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
				Model:     "claude-sonnet-4-20250514",
				APIKeyEnv: "ANTHROPIC_API_KEY",
				MaxTokens: 4096,
			},
			expectedConfig: &Config{
				Enabled:   false,
				Model:     "claude-sonnet-4-20250514",
				APIKeyEnv: "ANTHROPIC_API_KEY",
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
				Model:     "gpt-4o",
				APIKeyEnv: "OPENAI_API_KEY",
				MaxTokens: 4096,
			},
			expectedConfig: &Config{
				Enabled:   false,
				Model:     "gpt-4o",
				APIKeyEnv: "OPENAI_API_KEY",
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
				Model:     "gemini-2.0-flash-exp",
				APIKeyEnv: "GEMINI_API_KEY",
				MaxTokens: 8192,
			},
			expectedConfig: &Config{
				Enabled:   false,
				Model:     "gemini-2.0-flash-exp",
				APIKeyEnv: "GEMINI_API_KEY",
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
				Model:     "grok-4-latest",
				APIKeyEnv: "XAI_API_KEY",
				MaxTokens: 4096,
				BaseURL:   "https://api.x.ai/v1",
			},
			expectedConfig: &Config{
				Enabled:   true,
				Model:     "grok-4-latest",
				APIKeyEnv: "XAI_API_KEY",
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
