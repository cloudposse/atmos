package gemini

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/cloudposse/atmos/pkg/ai/agent/base"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
)

// newClientAgainst returns a Client whose requests go to a test server that records the
// generationConfig of the last request body.
func newClientAgainst(t *testing.T, maxTokens int, generationConfig *map[string]any) *Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var request map[string]any
		require.NoError(t, json.Unmarshal(body, &request))
		*generationConfig, _ = request["generationConfig"].(map[string]any)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`))
	}))
	t.Cleanup(server.Close)

	genaiClient, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:      "test-key",
		Backend:     genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL},
	})
	require.NoError(t, err)

	return &Client{
		client: genaiClient,
		config: &base.Config{Enabled: true, Model: DefaultModel, APIKey: "test-key", MaxTokens: maxTokens},
	}
}

func TestClient_SendsConfiguredMaxTokens(t *testing.T) {
	messages := []types.Message{{Role: types.RoleUser, Content: "hi"}}
	tool := &mockTool{name: "list_stacks", description: "List stacks"}

	sends := map[string]func(c *Client) error{
		"SendMessage": func(c *Client) error {
			_, err := c.SendMessage(context.Background(), "hi")
			return err
		},
		"SendMessageWithHistory": func(c *Client) error {
			_, err := c.SendMessageWithHistory(context.Background(), messages)
			return err
		},
		"SendMessageWithTools": func(c *Client) error {
			_, err := c.SendMessageWithTools(context.Background(), "hi", []tools.Tool{tool})
			return err
		},
		"SendMessageWithToolsAndHistory": func(c *Client) error {
			_, err := c.SendMessageWithToolsAndHistory(context.Background(), messages, []tools.Tool{tool})
			return err
		},
	}

	for name, send := range sends {
		t.Run(name, func(t *testing.T) {
			var generationConfig map[string]any
			client := newClientAgainst(t, 1234, &generationConfig)

			require.NoError(t, send(client))

			assert.EqualValues(t, 1234, generationConfig["maxOutputTokens"])
		})
	}
}
