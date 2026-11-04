package bedrock

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/agent/base"
	"github.com/cloudposse/atmos/pkg/ai/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// newCapturingClient returns a client whose InvokeModel calls go to a local server that records the request body.
func newCapturingClient(t *testing.T, cache *cacheConfig, response string) (*Client, *map[string]interface{}) {
	t.Helper()

	requestBody := map[string]interface{}{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&requestBody))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(server.Close)

	runtimeClient := bedrockruntime.NewFromConfig(aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("id", "secret", ""),
	}, func(o *bedrockruntime.Options) {
		o.BaseEndpoint = aws.String(server.URL)
	})

	return &Client{
		client: runtimeClient,
		config: &base.Config{Enabled: true, Model: DefaultModel, MaxTokens: DefaultMaxTokens},
		region: "us-east-1",
		cache:  cache,
	}, &requestBody
}

const okResponse = `{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`

func TestSendMessageWithSystemPromptAndTools_SendsSystemField(t *testing.T) {
	ephemeral := map[string]interface{}{"type": "ephemeral"}

	tests := []struct {
		name        string
		cache       *cacheConfig
		system      string
		memory      string
		wantBlocks  []map[string]interface{}
		wantNoField bool
	}{
		{
			name:   "default settings cache both blocks",
			cache:  nil,
			system: "You are the Atmos assistant.",
			memory: "# ATMOS.md",
			wantBlocks: []map[string]interface{}{
				{"type": "text", "text": "You are the Atmos assistant.", "cache_control": ephemeral},
				{"type": "text", "text": "# ATMOS.md", "cache_control": ephemeral},
			},
		},
		{
			name:   "caching disabled still sends the system prompt",
			cache:  &cacheConfig{enabled: false},
			system: "You are the Atmos assistant.",
			memory: "# ATMOS.md",
			wantBlocks: []map[string]interface{}{
				{"type": "text", "text": "You are the Atmos assistant."},
				{"type": "text", "text": "# ATMOS.md"},
			},
		},
		{
			name:   "only the system prompt is cached",
			cache:  &cacheConfig{enabled: true, cacheSystemPrompt: true, cacheProjectInstructions: false},
			system: "You are the Atmos assistant.",
			memory: "# ATMOS.md",
			wantBlocks: []map[string]interface{}{
				{"type": "text", "text": "You are the Atmos assistant.", "cache_control": ephemeral},
				{"type": "text", "text": "# ATMOS.md"},
			},
		},
		{
			name:   "no ATMOS.md sends one block",
			cache:  nil,
			system: "You are the Atmos assistant.",
			memory: "",
			wantBlocks: []map[string]interface{}{
				{"type": "text", "text": "You are the Atmos assistant.", "cache_control": ephemeral},
			},
		},
		{
			name:        "no system prompt and no ATMOS.md sends no system field",
			cache:       nil,
			wantNoField: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, requestBody := newCapturingClient(t, tt.cache, okResponse)
			history := []types.Message{
				{Role: types.RoleUser, Content: "list stacks"},
				{Role: types.RoleAssistant, Content: "Here they are."},
				{Role: types.RoleUser, Content: "and components?"},
			}

			result, err := client.SendMessageWithSystemPromptAndTools(context.Background(), tt.system, tt.memory, history, nil)

			require.NoError(t, err)
			assert.Equal(t, "ok", result.Content)

			// The messages array keeps user and assistant turns only, in order.
			messages, ok := (*requestBody)["messages"].([]interface{})
			require.True(t, ok)
			require.Len(t, messages, 3)
			assert.Equal(t, map[string]interface{}{"role": "user", "content": "list stacks"}, messages[0])
			assert.Equal(t, map[string]interface{}{"role": "user", "content": "and components?"}, messages[2])

			if tt.wantNoField {
				assert.NotContains(t, *requestBody, "system")
				return
			}
			system, ok := (*requestBody)["system"].([]interface{})
			require.True(t, ok, "request must carry a system field")
			require.Len(t, system, len(tt.wantBlocks))
			for i, want := range tt.wantBlocks {
				assert.Equal(t, want, system[i])
			}
		})
	}
}

func TestSendMessageWithToolsAndHistory_NoSystemField(t *testing.T) {
	client, requestBody := newCapturingClient(t, nil, okResponse)

	_, err := client.SendMessageWithToolsAndHistory(context.Background(), []types.Message{{Role: types.RoleUser, Content: "hi"}}, nil)

	require.NoError(t, err)
	assert.NotContains(t, *requestBody, "system")
}

func TestParseBedrockResponse_CacheUsage(t *testing.T) {
	responseBody := `{
		"content": [{"type": "text", "text": "ok"}],
		"stop_reason": "end_turn",
		"usage": {"input_tokens": 90, "output_tokens": 12, "cache_read_input_tokens": 5526, "cache_creation_input_tokens": 300}
	}`

	result, err := parseBedrockResponse([]byte(responseBody))

	require.NoError(t, err)
	require.NotNil(t, result.Usage)
	assert.Equal(t, int64(90), result.Usage.InputTokens)
	assert.Equal(t, int64(12), result.Usage.OutputTokens)
	assert.Equal(t, int64(102), result.Usage.TotalTokens)
	assert.Equal(t, int64(5526), result.Usage.CacheReadTokens)
	assert.Equal(t, int64(300), result.Usage.CacheCreationTokens)
}

func TestExtractCacheConfig(t *testing.T) {
	tests := []struct {
		name     string
		settings *schema.AICacheSettings
		want     cacheConfig
	}{
		{
			name:     "no cache settings keep caching on",
			settings: nil,
			want:     cacheConfig{enabled: true, cacheSystemPrompt: true, cacheProjectInstructions: true},
		},
		{
			name:     "disabled",
			settings: &schema.AICacheSettings{Enabled: false},
			want:     cacheConfig{enabled: false},
		},
		{
			name:     "enabled without fine-grained settings caches both",
			settings: &schema.AICacheSettings{Enabled: true},
			want:     cacheConfig{enabled: true, cacheSystemPrompt: true, cacheProjectInstructions: true},
		},
		{
			name:     "enabled with system prompt only",
			settings: &schema.AICacheSettings{Enabled: true, CacheSystemPrompt: true},
			want:     cacheConfig{enabled: true, cacheSystemPrompt: true, cacheProjectInstructions: false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{}
			atmosConfig.AI.Providers = map[string]*schema.AIProviderConfig{
				ProviderName: {Cache: tt.settings},
			}

			got := extractCacheConfig(atmosConfig)

			require.NotNil(t, got)
			assert.Equal(t, tt.want, *got)
		})
	}
}

func TestExtractCacheConfig_NoProviderConfig(t *testing.T) {
	got := extractCacheConfig(&schema.AtmosConfiguration{})

	require.NotNil(t, got)
	assert.Equal(t, *defaultCacheConfig(), *got)
}
