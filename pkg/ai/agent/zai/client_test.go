//nolint:dupl // Test files across OpenAI-compatible providers share similar setup by design for isolation and clarity.
package zai

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

func TestDefaultConstants(t *testing.T) {
	assert.Equal(t, "zai", ProviderName)
	assert.Equal(t, 4096, DefaultMaxTokens)
	assert.Equal(t, "glm-5.3", DefaultModel)
	assert.Equal(t, "ZAI_API_KEY", DefaultAPIKeyEnvVar)
	assert.Equal(t, "https://api.z.ai/api/paas/v4", DefaultBaseURL)
	assert.True(t, strings.HasPrefix(DefaultBaseURL, "https://"))
}

func TestExtractConfig(t *testing.T) {
	tests := []struct {
		name           string
		atmosConfig    *schema.AtmosConfiguration
		expectedConfig *base.Config
	}{
		{
			name:        "Default configuration",
			atmosConfig: &schema.AtmosConfiguration{AI: schema.AISettings{}},
			expectedConfig: &base.Config{
				Enabled:   false,
				Model:     DefaultModel,
				APIKey:    "",
				MaxTokens: DefaultMaxTokens,
				BaseURL:   DefaultBaseURL,
			},
		},
		{
			name: "All overrides",
			atmosConfig: &schema.AtmosConfiguration{
				AI: schema.AISettings{
					Enabled: true,
					Providers: map[string]*schema.AIProviderConfig{
						"zai": {
							Model:     "glm-5.3-flash",
							ApiKey:    "custom-zai-key",
							MaxTokens: 8192,
							BaseURL:   "https://proxy.example.com/api/paas/v4",
						},
					},
				},
			},
			expectedConfig: &base.Config{
				Enabled:   true,
				Model:     "glm-5.3-flash",
				APIKey:    "custom-zai-key",
				MaxTokens: 8192,
				BaseURL:   "https://proxy.example.com/api/paas/v4",
			},
		},
		{
			name: "Partial configuration uses defaults",
			atmosConfig: &schema.AtmosConfiguration{
				AI: schema.AISettings{
					Enabled: true,
					Providers: map[string]*schema.AIProviderConfig{
						"zai": {Model: "glm-5.3-flash"},
					},
				},
			},
			expectedConfig: &base.Config{
				Enabled:   true,
				Model:     "glm-5.3-flash",
				APIKey:    "",
				MaxTokens: DefaultMaxTokens,
				BaseURL:   DefaultBaseURL,
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
	client, err := NewClient(&schema.AtmosConfiguration{AI: schema.AISettings{Enabled: false}})
	assert.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "AI features are disabled")
}

func TestNewClient_MissingAPIKey(t *testing.T) {
	client, err := NewClient(&schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled:   true,
			Providers: map[string]*schema.AIProviderConfig{"zai": {}},
		},
	})
	assert.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "API key not found")
}

func TestNewClient_Valid(t *testing.T) {
	client, err := NewClient(&schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled:   true,
			Providers: map[string]*schema.AIProviderConfig{"zai": {ApiKey: "test-key"}},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, client)
	assert.Equal(t, DefaultModel, client.GetModel())
	assert.Equal(t, DefaultMaxTokens, client.GetMaxTokens())
	assert.Equal(t, DefaultBaseURL, client.GetBaseURL())
}

func TestClientGetters(t *testing.T) {
	client := &Client{config: &base.Config{Model: "glm-5.3-flash", MaxTokens: 1234, BaseURL: "https://x"}}
	assert.Equal(t, "glm-5.3-flash", client.GetModel())
	assert.Equal(t, 1234, client.GetMaxTokens())
	assert.Equal(t, "https://x", client.GetBaseURL())
}

func TestClientSendMethods(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Contains(t, r.URL.Path, "/chat/completions")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-test","object":"chat.completion","created":0,"model":"glm-5.3",
			"choices":[{"index":0,"message":{"role":"assistant","content":"hello from zai"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":7,"completion_tokens":8,"total_tokens":15}
		}`))
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(&schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled: true,
			Providers: map[string]*schema.AIProviderConfig{
				"zai": {ApiKey: "test-key", BaseURL: server.URL, MaxTokens: 128},
			},
		},
	})
	require.NoError(t, err)

	ctx := context.Background()
	messages := []types.Message{{Role: types.RoleUser, Content: "hello"}}

	text, err := client.SendMessage(ctx, "hello")
	require.NoError(t, err)
	assert.Equal(t, "hello from zai", text)

	text, err = client.SendMessageWithHistory(ctx, messages)
	require.NoError(t, err)
	assert.Equal(t, "hello from zai", text)

	response, err := client.SendMessageWithTools(ctx, "hello", nil)
	require.NoError(t, err)
	assert.Equal(t, "hello from zai", response.Content)
	require.NotNil(t, response.Usage)
	assert.Equal(t, int64(15), response.Usage.TotalTokens)

	response, err = client.SendMessageWithToolsAndHistory(ctx, messages, nil)
	require.NoError(t, err)
	assert.Equal(t, types.StopReasonEndTurn, response.StopReason)

	response, err = client.SendMessageWithSystemPromptAndTools(ctx, "system", "memory", messages, nil)
	require.NoError(t, err)
	assert.Equal(t, "hello from zai", response.Content)
}
