package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/agent/base"
	"github.com/cloudposse/atmos/pkg/ai/types"
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
				AI: schema.AISettings{},
			},
			expectedConfig: &base.Config{
				Enabled:   false,
				Model:     "openai/gpt-4o-mini",
				APIKey:    "",
				MaxTokens: 4096,
				BaseURL:   "https://models.github.ai/inference",
			},
		},
		{
			name: "Enabled configuration",
			atmosConfig: &schema.AtmosConfiguration{
				AI: schema.AISettings{
					Enabled: true,
					Providers: map[string]*schema.AIProviderConfig{
						"github": {
							Model:     "openai/gpt-4o",
							ApiKey:    "custom-github-token-value",
							MaxTokens: 8192,
							BaseURL:   "https://models.example.ghe.com/inference",
						},
					},
				},
			},
			expectedConfig: &base.Config{
				Enabled:   true,
				Model:     "openai/gpt-4o",
				APIKey:    "custom-github-token-value",
				MaxTokens: 8192,
				BaseURL:   "https://models.example.ghe.com/inference",
			},
		},
		{
			name: "Partial configuration",
			atmosConfig: &schema.AtmosConfiguration{
				AI: schema.AISettings{
					Enabled: true,
					Providers: map[string]*schema.AIProviderConfig{
						"github": {
							Model: "meta/llama-3.3-70b-instruct",
						},
					},
				},
			},
			expectedConfig: &base.Config{
				Enabled:   true,
				Model:     "meta/llama-3.3-70b-instruct",
				APIKey:    "",
				MaxTokens: 4096,
				BaseURL:   "https://models.github.ai/inference",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := base.ExtractConfig(tt.atmosConfig, ProviderName, base.ProviderDefaults{
				Model:         DefaultModel,
				DefaultAPIKey: "",
				MaxTokens:     DefaultMaxTokens,
				BaseURL:       DefaultBaseURL,
			})
			assert.Equal(t, tt.expectedConfig, config)
		})
	}
}

func TestNewClient_Disabled(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled: false,
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
		Model:     "openai/gpt-4o-mini",
		APIKey:    "",
		MaxTokens: 4096,
		BaseURL:   "https://models.github.ai/inference",
	}

	client := &Client{
		client: nil, // We don't need a real client for testing getters.
		config: config,
	}

	assert.Equal(t, "openai/gpt-4o-mini", client.GetModel())
	assert.Equal(t, 4096, client.GetMaxTokens())
	assert.Equal(t, "https://models.github.ai/inference", client.GetBaseURL())
}

func TestNewClient_MissingAPIKey(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled: true,
			Providers: map[string]*schema.AIProviderConfig{
				"github": {
					// ApiKey is empty - should fail.
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
	assert.Equal(t, "github", ProviderName)
	assert.Equal(t, 4096, DefaultMaxTokens)
	assert.Equal(t, "openai/gpt-4o-mini", DefaultModel)
	assert.Equal(t, "GITHUB_TOKEN", DefaultAPIKeyEnvVar)
	assert.Equal(t, "https://models.github.ai/inference", DefaultBaseURL)
}

func TestExtractConfig_NilProviders(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled:   true,
			Providers: nil,
		},
	}

	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:         DefaultModel,
		DefaultAPIKey: "",
		MaxTokens:     DefaultMaxTokens,
		BaseURL:       DefaultBaseURL,
	})

	// Should use defaults when providers is nil.
	assert.True(t, config.Enabled)
	assert.Equal(t, DefaultModel, config.Model)
	assert.Empty(t, config.APIKey)
	assert.Equal(t, DefaultMaxTokens, config.MaxTokens)
	assert.Equal(t, DefaultBaseURL, config.BaseURL)
}

func TestExtractConfig_DifferentProviderOnly(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled: true,
			Providers: map[string]*schema.AIProviderConfig{
				"openai": {
					Model: "gpt-4o",
				},
			},
		},
	}

	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:         DefaultModel,
		DefaultAPIKey: "",
		MaxTokens:     DefaultMaxTokens,
		BaseURL:       DefaultBaseURL,
	})

	// Should use defaults when this provider is not configured.
	assert.True(t, config.Enabled)
	assert.Equal(t, DefaultModel, config.Model)
	assert.Empty(t, config.APIKey)
	assert.Equal(t, DefaultMaxTokens, config.MaxTokens)
	assert.Equal(t, DefaultBaseURL, config.BaseURL)
}

func TestExtractConfig_NilProviderConfig(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled: true,
			Providers: map[string]*schema.AIProviderConfig{
				"github": nil, // Explicitly nil provider config.
			},
		},
	}

	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:         DefaultModel,
		DefaultAPIKey: "",
		MaxTokens:     DefaultMaxTokens,
		BaseURL:       DefaultBaseURL,
	})

	// Should use defaults when provider config is nil.
	assert.True(t, config.Enabled)
	assert.Equal(t, DefaultModel, config.Model)
	assert.Empty(t, config.APIKey)
	assert.Equal(t, DefaultMaxTokens, config.MaxTokens)
	assert.Equal(t, DefaultBaseURL, config.BaseURL)
}

func TestExtractConfig_CustomBaseURL(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled: true,
			Providers: map[string]*schema.AIProviderConfig{
				"github": {
					BaseURL: "https://models.ghe.example.com/inference",
				},
			},
		},
	}

	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:         DefaultModel,
		DefaultAPIKey: "",
		MaxTokens:     DefaultMaxTokens,
		BaseURL:       DefaultBaseURL,
	})

	assert.Equal(t, "https://models.ghe.example.com/inference", config.BaseURL)
}

func TestExtractConfig_AllOverrides(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled: true,
			Providers: map[string]*schema.AIProviderConfig{
				"github": {
					Model:     "openai/gpt-4.1",
					ApiKey:    "custom-github-token-value",
					MaxTokens: 32768,
					BaseURL:   "https://models.custom.example.com/inference",
				},
			},
		},
	}

	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:         DefaultModel,
		DefaultAPIKey: "",
		MaxTokens:     DefaultMaxTokens,
		BaseURL:       DefaultBaseURL,
	})

	assert.True(t, config.Enabled)
	assert.Equal(t, "openai/gpt-4.1", config.Model)
	assert.Equal(t, "custom-github-token-value", config.APIKey)
	assert.Equal(t, 32768, config.MaxTokens)
	assert.Equal(t, "https://models.custom.example.com/inference", config.BaseURL)
}

func TestNewClient_WithValidAPIKey(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled: true,
			Providers: map[string]*schema.AIProviderConfig{
				"github": {
					ApiKey: "test-github-token-value",
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
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled: true,
			Providers: map[string]*schema.AIProviderConfig{
				"github": {
					Model:     "openai/gpt-4o",
					ApiKey:    "custom-token",
					MaxTokens: 8192,
					BaseURL:   "https://models.custom.github.ai/inference",
				},
			},
		},
	}

	client, err := NewClient(atmosConfig)
	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, "openai/gpt-4o", client.GetModel())
	assert.Equal(t, 8192, client.GetMaxTokens())
	assert.Equal(t, "https://models.custom.github.ai/inference", client.GetBaseURL())
}

func TestClientSendMethods(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Contains(t, r.URL.Path, "/chat/completions")
		// GitHub Models authenticates with a bearer token (GITHUB_TOKEN in CI).
		assert.Equal(t, "Bearer test-github-token-value", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-test",
			"object":"chat.completion",
			"created":0,
			"model":"openai/gpt-4o-mini",
			"choices":[{
				"index":0,
				"message":{"role":"assistant","content":"hello from github models"},
				"finish_reason":"stop"
			}],
			"usage":{"prompt_tokens":7,"completion_tokens":8,"total_tokens":15}
		}`))
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(&schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled: true,
			Providers: map[string]*schema.AIProviderConfig{
				"github": {
					ApiKey:    "test-github-token-value",
					BaseURL:   server.URL,
					MaxTokens: 128,
				},
			},
		},
	})
	require.NoError(t, err)

	ctx := context.Background()
	messages := []types.Message{{Role: types.RoleUser, Content: "hello"}}

	text, err := client.SendMessage(ctx, "hello")
	require.NoError(t, err)
	assert.Equal(t, "hello from github models", text)

	text, err = client.SendMessageWithHistory(ctx, messages)
	require.NoError(t, err)
	assert.Equal(t, "hello from github models", text)

	response, err := client.SendMessageWithTools(ctx, "hello", nil)
	require.NoError(t, err)
	assert.Equal(t, "hello from github models", response.Content)
	require.NotNil(t, response.Usage)
	assert.Equal(t, int64(15), response.Usage.TotalTokens)

	response, err = client.SendMessageWithToolsAndHistory(ctx, messages, nil)
	require.NoError(t, err)
	assert.Equal(t, types.StopReasonEndTurn, response.StopReason)

	response, err = client.SendMessageWithSystemPromptAndTools(ctx, "system", "memory", messages, nil)
	require.NoError(t, err)
	assert.Equal(t, "hello from github models", response.Content)
}

func TestExtractConfig_EdgeCases(t *testing.T) {
	// Test with MaxTokens = 0 (should use default).
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled: true,
			Providers: map[string]*schema.AIProviderConfig{
				"github": {
					MaxTokens: 0, // Zero should not override default.
				},
			},
		},
	}

	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:         DefaultModel,
		DefaultAPIKey: "",
		MaxTokens:     DefaultMaxTokens,
		BaseURL:       DefaultBaseURL,
	})

	// MaxTokens should use default since 0 is not > 0.
	assert.Equal(t, DefaultMaxTokens, config.MaxTokens)
}

func TestExtractConfig_PartialOverrides(t *testing.T) {
	tests := []struct {
		name           string
		providerConfig *schema.AIProviderConfig
		expectedModel  string
		expectedKey    string
		expectedTokens int
		expectedURL    string
	}{
		{
			name: "Only model override",
			providerConfig: &schema.AIProviderConfig{
				Model: "mistral-ai/mistral-large-2411",
			},
			expectedModel:  "mistral-ai/mistral-large-2411",
			expectedKey:    "",
			expectedTokens: DefaultMaxTokens,
			expectedURL:    DefaultBaseURL,
		},
		{
			name: "Only API key override",
			providerConfig: &schema.AIProviderConfig{
				ApiKey: "custom-token-value",
			},
			expectedModel:  DefaultModel,
			expectedKey:    "custom-token-value",
			expectedTokens: DefaultMaxTokens,
			expectedURL:    DefaultBaseURL,
		},
		{
			name: "Only max tokens override",
			providerConfig: &schema.AIProviderConfig{
				MaxTokens: 10000,
			},
			expectedModel:  DefaultModel,
			expectedKey:    "",
			expectedTokens: 10000,
			expectedURL:    DefaultBaseURL,
		},
		{
			name: "Only base URL override",
			providerConfig: &schema.AIProviderConfig{
				BaseURL: "https://proxy.example.com/github-models",
			},
			expectedModel:  DefaultModel,
			expectedKey:    "",
			expectedTokens: DefaultMaxTokens,
			expectedURL:    "https://proxy.example.com/github-models",
		},
		{
			name: "Model and tokens override",
			providerConfig: &schema.AIProviderConfig{
				Model:     "openai/gpt-4o",
				MaxTokens: 16000,
			},
			expectedModel:  "openai/gpt-4o",
			expectedKey:    "",
			expectedTokens: 16000,
			expectedURL:    DefaultBaseURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				AI: schema.AISettings{
					Enabled: true,
					Providers: map[string]*schema.AIProviderConfig{
						"github": tt.providerConfig,
					},
				},
			}

			config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
				Model:         DefaultModel,
				DefaultAPIKey: "",
				MaxTokens:     DefaultMaxTokens,
				BaseURL:       DefaultBaseURL,
			})

			assert.Equal(t, tt.expectedModel, config.Model)
			assert.Equal(t, tt.expectedKey, config.APIKey)
			assert.Equal(t, tt.expectedTokens, config.MaxTokens)
			assert.Equal(t, tt.expectedURL, config.BaseURL)
		})
	}
}

func TestProviderName_Constant(t *testing.T) {
	assert.Equal(t, "github", ProviderName)
	// Verify it's a lowercase string suitable for config lookups.
	assert.NotContains(t, ProviderName, " ")
	assert.Equal(t, strings.ToLower(ProviderName), ProviderName)
}

func TestDefaultValues_AllConstants(t *testing.T) {
	// DefaultMaxTokens should be reasonable for most use cases.
	assert.Greater(t, DefaultMaxTokens, 0)
	assert.LessOrEqual(t, DefaultMaxTokens, 200000)

	// DefaultModel should use the publisher/model-name format used by GitHub Models.
	assert.NotEmpty(t, DefaultModel)
	assert.Contains(t, DefaultModel, "/")

	// DefaultAPIKeyEnvVar should be the built-in GitHub Actions token variable.
	assert.Equal(t, "GITHUB_TOKEN", DefaultAPIKeyEnvVar)

	// DefaultBaseURL should be GitHub Models' official inference endpoint.
	assert.NotEmpty(t, DefaultBaseURL)
	assert.Contains(t, DefaultBaseURL, "models.github.ai")
	assert.True(t, strings.HasPrefix(DefaultBaseURL, "https://"), "BaseURL should start with https://")
}

func TestNewClient_NilAtmosConfig(t *testing.T) {
	// Test with empty AtmosConfiguration - should not panic.
	// The ExtractConfig function handles missing sections safely.
	atmosConfig := &schema.AtmosConfiguration{}

	client, err := NewClient(atmosConfig)
	assert.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "AI features are disabled")
}

func TestGitHubModels_VariousPublishers(t *testing.T) {
	// GitHub Models namespaces models by publisher (openai/, meta/, mistral-ai/, ...).
	models := []string{
		"openai/gpt-4o-mini",
		"openai/gpt-4o",
		"openai/gpt-4.1",
		"meta/llama-3.3-70b-instruct",
		"mistral-ai/mistral-large-2411",
		"deepseek/deepseek-r1",
	}

	for _, modelID := range models {
		t.Run(modelID, func(t *testing.T) {
			client := &Client{
				client: nil,
				config: &base.Config{
					Enabled:   true,
					Model:     modelID,
					APIKey:    "",
					MaxTokens: DefaultMaxTokens,
					BaseURL:   DefaultBaseURL,
				},
			}

			assert.Equal(t, modelID, client.GetModel())
		})
	}
}

func TestExtractConfig_MultipleProviders(t *testing.T) {
	// Test that GitHub Models config is extracted correctly when multiple providers are configured.
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled: true,
			Providers: map[string]*schema.AIProviderConfig{
				"openai": {
					Model: "gpt-4o",
				},
				"github": {
					Model:     "openai/gpt-4o",
					MaxTokens: 8192,
				},
				"anthropic": {
					Model: "claude-sonnet-4-5-20250929",
				},
			},
		},
	}

	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:         DefaultModel,
		DefaultAPIKey: "",
		MaxTokens:     DefaultMaxTokens,
		BaseURL:       DefaultBaseURL,
	})

	// Should extract only the GitHub Models config, not others.
	assert.True(t, config.Enabled)
	assert.Equal(t, "openai/gpt-4o", config.Model)
	assert.Equal(t, 8192, config.MaxTokens)
	assert.Empty(t, config.APIKey)
	assert.Equal(t, DefaultBaseURL, config.BaseURL)
}
