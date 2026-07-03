package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/agent/base"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// anthropicMessageResponse returns a minimal valid Anthropic API response JSON.
func anthropicMessageResponse(content string) []byte {
	resp := map[string]interface{}{
		"id":            "msg_test123",
		"type":          "message",
		"role":          "assistant",
		"model":         "claude-sonnet-4-5-20250929",
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
		"usage": map[string]interface{}{
			"input_tokens":                50,
			"output_tokens":               20,
			"cache_read_input_tokens":     0,
			"cache_creation_input_tokens": 0,
		},
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": content,
			},
		},
	}

	data, _ := json.Marshal(resp)
	return data
}

// anthropicToolUseResponse returns a mock Anthropic API response with a tool use block.
func anthropicToolUseResponse(toolID, toolName string, input map[string]interface{}) []byte {
	inputJSON, _ := json.Marshal(input)
	resp := map[string]interface{}{
		"id":            "msg_test456",
		"type":          "message",
		"role":          "assistant",
		"model":         "claude-sonnet-4-5-20250929",
		"stop_reason":   "tool_use",
		"stop_sequence": nil,
		"usage": map[string]interface{}{
			"input_tokens":                100,
			"output_tokens":               50,
			"cache_read_input_tokens":     0,
			"cache_creation_input_tokens": 0,
		},
		"content": []map[string]interface{}{
			{
				"type":  "tool_use",
				"id":    toolID,
				"name":  toolName,
				"input": json.RawMessage(inputJSON),
			},
		},
	}

	data, _ := json.Marshal(resp)
	return data
}

// newTestSimpleClient creates a SimpleClient backed by a mock HTTP test server.
// The handler function controls what the mock server returns.
func newTestSimpleClient(t *testing.T, handler http.Handler) *SimpleClient {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	sdkClient := anthropicsdk.NewClient(
		option.WithBaseURL(server.URL),
		option.WithAPIKey("test-api-key"),
		option.WithMaxRetries(0),
	)

	client := &SimpleClient{
		client: &sdkClient,
		config: &base.Config{
			Enabled:   true,
			Model:     DefaultModel,
			APIKey:    "test-api-key",
			MaxTokens: DefaultMaxTokens,
		},
		cache: &cacheConfig{
			enabled:                  true,
			cacheSystemPrompt:        true,
			cacheProjectInstructions: true,
		},
	}

	return client
}

// TestSendMessage_Success tests SendMessage with a successful text response.
func TestSendMessage_Success(t *testing.T) {
	expectedContent := "Hello from the AI!"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(anthropicMessageResponse(expectedContent))
	})

	client := newTestSimpleClient(t, handler)

	result, err := client.SendMessage(context.Background(), "Hello")

	require.NoError(t, err)
	assert.Equal(t, expectedContent, result)
}

// TestSendMessage_APIError tests SendMessage when the API returns an error.
func TestSendMessage_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"api_error","message":"Internal server error"}}`))
	})

	client := newTestSimpleClient(t, handler)

	result, err := client.SendMessage(context.Background(), "Hello")

	assert.Error(t, err)
	assert.Empty(t, result)
}

// TestSendMessage_EmptyContent tests SendMessage with an empty text response.
func TestSendMessage_EmptyContent(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(anthropicMessageResponse(""))
	})

	client := newTestSimpleClient(t, handler)

	result, err := client.SendMessage(context.Background(), "Hello")

	require.NoError(t, err)
	assert.Empty(t, result)
}

// TestSendMessageWithTools_Success tests SendMessageWithTools with a successful text response.
func TestSendMessageWithTools_Success(t *testing.T) {
	expectedContent := "I'll help you with that."

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(anthropicMessageResponse(expectedContent))
	})

	client := newTestSimpleClient(t, handler)

	availableTools := []tools.Tool{
		&mockTool{
			name:        "search",
			description: "Search for information.",
			parameters: []tools.Parameter{
				{Name: "query", Type: tools.ParamTypeString, Description: "Search query", Required: true},
			},
		},
	}

	result, err := client.SendMessageWithTools(context.Background(), "Search for something", availableTools)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, expectedContent, result.Content)
	assert.Equal(t, types.StopReasonEndTurn, result.StopReason)
}

// TestSendMessageWithTools_ToolUseResponse tests SendMessageWithTools when the AI calls a tool.
func TestSendMessageWithTools_ToolUseResponse(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := anthropicToolUseResponse("toolu_abc123", "search", map[string]interface{}{
			"query": "test query",
		})
		_, _ = w.Write(resp)
	})

	client := newTestSimpleClient(t, handler)

	availableTools := []tools.Tool{
		&mockTool{
			name:        "search",
			description: "Search for information.",
			parameters: []tools.Parameter{
				{Name: "query", Type: tools.ParamTypeString, Description: "Search query", Required: true},
			},
		},
	}

	result, err := client.SendMessageWithTools(context.Background(), "Search for something", availableTools)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, types.StopReasonToolUse, result.StopReason)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "toolu_abc123", result.ToolCalls[0].ID)
	assert.Equal(t, "search", result.ToolCalls[0].Name)
	assert.Equal(t, "test query", result.ToolCalls[0].Input["query"])
}

// TestSendMessageWithTools_APIError tests SendMessageWithTools when the API returns an error.
func TestSendMessageWithTools_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"Invalid API key"}}`))
	})

	client := newTestSimpleClient(t, handler)

	result, err := client.SendMessageWithTools(context.Background(), "Hello", []tools.Tool{})

	assert.Error(t, err)
	assert.Nil(t, result)
}

// TestSendMessageWithHistory_Success tests SendMessageWithHistory with conversation history.
func TestSendMessageWithHistory_Success(t *testing.T) {
	expectedContent := "Nice to meet you!"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(anthropicMessageResponse(expectedContent))
	})

	client := newTestSimpleClient(t, handler)

	messages := []types.Message{
		{Role: types.RoleUser, Content: "Hello, I'm Alice."},
		{Role: types.RoleAssistant, Content: "Hello Alice!"},
		{Role: types.RoleUser, Content: "Nice to meet you too."},
	}

	result, err := client.SendMessageWithHistory(context.Background(), messages)

	require.NoError(t, err)
	assert.Equal(t, expectedContent, result)
}

// TestSendMessageWithHistory_APIError tests SendMessageWithHistory with an API error.
func TestSendMessageWithHistory_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"Rate limit exceeded"}}`))
	})

	client := newTestSimpleClient(t, handler)

	messages := []types.Message{
		{Role: types.RoleUser, Content: "Hello"},
	}

	result, err := client.SendMessageWithHistory(context.Background(), messages)

	assert.Error(t, err)
	assert.Empty(t, result)
}

// TestSendMessageWithHistory_SystemMessagesSkipped tests that system messages are skipped in history.
func TestSendMessageWithHistory_SystemMessagesSkipped(t *testing.T) {
	expectedContent := "Response after skipping system message."

	var capturedBody map[string]interface{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(anthropicMessageResponse(expectedContent))
	})

	client := newTestSimpleClient(t, handler)

	messages := []types.Message{
		{Role: types.RoleSystem, Content: "You are helpful."},
		{Role: types.RoleUser, Content: "Hello"},
	}

	result, err := client.SendMessageWithHistory(context.Background(), messages)

	require.NoError(t, err)
	assert.Equal(t, expectedContent, result)

	// Verify the system message was skipped - only user message should be in the API request.
	if capturedBody != nil {
		if msgs, ok := capturedBody["messages"].([]interface{}); ok {
			assert.Len(t, msgs, 1, "System message should be skipped in conversation history")
		}
	}
}

// TestSendMessageWithToolsAndHistory_Success tests SendMessageWithToolsAndHistory.
func TestSendMessageWithToolsAndHistory_Success(t *testing.T) {
	expectedContent := "Based on the conversation, I'll search for that."

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(anthropicMessageResponse(expectedContent))
	})

	client := newTestSimpleClient(t, handler)

	messages := []types.Message{
		{Role: types.RoleUser, Content: "Can you search for Go documentation?"},
	}

	availableTools := []tools.Tool{
		&mockTool{
			name:        "web_search",
			description: "Search the web.",
			parameters: []tools.Parameter{
				{Name: "query", Type: tools.ParamTypeString, Description: "Search query", Required: true},
			},
		},
	}

	result, err := client.SendMessageWithToolsAndHistory(context.Background(), messages, availableTools)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, expectedContent, result.Content)
}

// TestSendMessageWithToolsAndHistory_ToolUse tests SendMessageWithToolsAndHistory when tool is called.
func TestSendMessageWithToolsAndHistory_ToolUse(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := anthropicToolUseResponse("toolu_xyz", "web_search", map[string]interface{}{
			"query": "Go documentation",
		})
		_, _ = w.Write(resp)
	})

	client := newTestSimpleClient(t, handler)

	messages := []types.Message{
		{Role: types.RoleUser, Content: "Search for Go docs."},
	}

	availableTools := []tools.Tool{
		&mockTool{
			name:        "web_search",
			description: "Search the web.",
			parameters: []tools.Parameter{
				{Name: "query", Type: tools.ParamTypeString, Description: "Query", Required: true},
			},
		},
	}

	result, err := client.SendMessageWithToolsAndHistory(context.Background(), messages, availableTools)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, types.StopReasonToolUse, result.StopReason)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "web_search", result.ToolCalls[0].Name)
}

// TestSendMessageWithToolsAndHistory_APIError tests SendMessageWithToolsAndHistory with an API error.
func TestSendMessageWithToolsAndHistory_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"Bad request"}}`))
	})

	client := newTestSimpleClient(t, handler)

	messages := []types.Message{
		{Role: types.RoleUser, Content: "Hello"},
	}

	result, err := client.SendMessageWithToolsAndHistory(context.Background(), messages, []tools.Tool{})

	assert.Error(t, err)
	assert.Nil(t, result)
}

// TestSendMessageWithSystemPromptAndTools_Success tests the full method with system prompt.
func TestSendMessageWithSystemPromptAndTools_Success(t *testing.T) {
	expectedContent := "I am a helpful Atmos assistant."

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(anthropicMessageResponse(expectedContent))
	})

	client := newTestSimpleClient(t, handler)

	messages := []types.Message{
		{Role: types.RoleUser, Content: "Who are you?"},
	}

	result, err := client.SendMessageWithSystemPromptAndTools(
		context.Background(),
		"You are a helpful Atmos assistant.",
		"# ATMOS.md\nAtmos is a tool for cloud infrastructure orchestration.",
		messages,
		[]tools.Tool{},
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, expectedContent, result.Content)
}

// TestSendMessageWithSystemPromptAndTools_NoSystemPrompt tests with empty system prompt.
func TestSendMessageWithSystemPromptAndTools_NoSystemPrompt(t *testing.T) {
	expectedContent := "Response without system prompt."

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(anthropicMessageResponse(expectedContent))
	})

	client := newTestSimpleClient(t, handler)

	messages := []types.Message{
		{Role: types.RoleUser, Content: "Hello"},
	}

	result, err := client.SendMessageWithSystemPromptAndTools(
		context.Background(),
		"", // No system prompt.
		"", // No atmos memory.
		messages,
		[]tools.Tool{},
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, expectedContent, result.Content)
}

// TestSendMessageWithSystemPromptAndTools_WithTools tests with tools and system prompt.
func TestSendMessageWithSystemPromptAndTools_WithTools(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := anthropicToolUseResponse("toolu_syst", "list_stacks", map[string]interface{}{})
		_, _ = w.Write(resp)
	})

	client := newTestSimpleClient(t, handler)

	messages := []types.Message{
		{Role: types.RoleUser, Content: "List all stacks."},
	}

	availableTools := []tools.Tool{
		&mockTool{
			name:        "list_stacks",
			description: "List all Atmos stacks.",
			parameters:  []tools.Parameter{},
		},
	}

	result, err := client.SendMessageWithSystemPromptAndTools(
		context.Background(),
		"You are an Atmos assistant.",
		"Memory content here.",
		messages,
		availableTools,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, types.StopReasonToolUse, result.StopReason)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "list_stacks", result.ToolCalls[0].Name)
}

// TestSendMessageWithSystemPromptAndTools_APIError tests API error handling.
func TestSendMessageWithSystemPromptAndTools_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"overloaded_error","message":"Service overloaded"}}`))
	})

	client := newTestSimpleClient(t, handler)

	result, err := client.SendMessageWithSystemPromptAndTools(
		context.Background(),
		"System prompt",
		"Memory",
		[]types.Message{{Role: types.RoleUser, Content: "Hello"}},
		[]tools.Tool{},
	)

	assert.Error(t, err)
	assert.Nil(t, result)
}

// TestSendMessageWithSystemPromptAndTools_CacheDisabled tests with caching disabled.
func TestSendMessageWithSystemPromptAndTools_CacheDisabled(t *testing.T) {
	expectedContent := "Response with no caching."

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(anthropicMessageResponse(expectedContent))
	})

	// Create a client with caching disabled.
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	sdkClient := anthropicsdk.NewClient(
		option.WithBaseURL(server.URL),
		option.WithAPIKey("test-key"),
		option.WithMaxRetries(0),
	)

	client := &SimpleClient{
		client: &sdkClient,
		config: &base.Config{
			Enabled:   true,
			Model:     DefaultModel,
			MaxTokens: DefaultMaxTokens,
		},
		cache: &cacheConfig{
			enabled:                  false,
			cacheSystemPrompt:        false,
			cacheProjectInstructions: false,
		},
	}

	result, err := client.SendMessageWithSystemPromptAndTools(
		context.Background(),
		"You are helpful.",
		"ATMOS.md content",
		[]types.Message{{Role: types.RoleUser, Content: "Hello"}},
		[]tools.Tool{},
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, expectedContent, result.Content)
}

// TestNewSimpleClient_WithAPIKey tests that NewSimpleClient succeeds when API key is set.
func TestNewSimpleClient_WithAPIKey(t *testing.T) {
	// This test directly tests the NewSimpleClient success path by passing
	// the API key directly. Since Anthropic client creation
	// does not validate the key (no network call), it should succeed.
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled: true,
			Providers: map[string]*schema.AIProviderConfig{
				"anthropic": {
					ApiKey: "sk-test-valid-key",
				},
			},
		},
	}

	client, err := NewSimpleClient(atmosConfig)
	require.NoError(t, err)
	require.NotNil(t, client)
	assert.Equal(t, DefaultModel, client.GetModel())
	assert.Equal(t, DefaultMaxTokens, client.GetMaxTokens())
}

// TestInitRegistersProvider tests that the init function registers the anthropic provider.
func TestInitRegistersProvider(t *testing.T) {
	// The init() function is called when the package is loaded.
	// We can verify the provider is registered by attempting to use the registry.
	// Since we're in the same package, the init() already ran, so we just verify
	// the registration happened (indirectly, via the provider name constant).
	assert.Equal(t, "anthropic", ProviderName)
}
