package executor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// recordingClient records what the executor sends to the provider.
type recordingClient struct {
	plainPrompts  []string
	historyPrompt []types.Message
	systemPrompt  string
}

func (c *recordingClient) SendMessage(_ context.Context, message string) (string, error) {
	c.plainPrompts = append(c.plainPrompts, message)
	return "ok", nil
}

func (c *recordingClient) SendMessageWithHistory(_ context.Context, messages []types.Message) (string, error) {
	c.historyPrompt = append([]types.Message(nil), messages...)
	return "ok", nil
}

func (c *recordingClient) SendMessageWithTools(context.Context, string, []tools.Tool) (*types.Response, error) {
	return &types.Response{Content: "ok", StopReason: types.StopReasonEndTurn}, nil
}

func (c *recordingClient) SendMessageWithToolsAndHistory(context.Context, []types.Message, []tools.Tool) (*types.Response, error) {
	return &types.Response{Content: "ok", StopReason: types.StopReasonEndTurn}, nil
}

func (c *recordingClient) SendMessageWithSystemPromptAndTools(_ context.Context, systemPrompt, _ string, _ []types.Message, _ []tools.Tool) (*types.Response, error) {
	c.systemPrompt = systemPrompt
	return &types.Response{Content: "ok", StopReason: types.StopReasonEndTurn}, nil
}

func (c *recordingClient) GetModel() string  { return "recording" }
func (c *recordingClient) GetMaxTokens() int { return 0 }

func TestResponseStyle_PlainPath(t *testing.T) {
	t.Run("the question is prefixed with the response style", func(t *testing.T) {
		client := &recordingClient{}
		result := NewExecutor(client, nil, &schema.AtmosConfiguration{}).Execute(t.Context(), Options{Prompt: "how many stacks?"})

		require.True(t, result.Success)
		require.Len(t, client.plainPrompts, 1)
		assert.Contains(t, client.plainPrompts[0], "GitHub-flavored Markdown")
		assert.Contains(t, client.plainPrompts[0], "end with at most one short")
		assert.Contains(t, client.plainPrompts[0], "---\n\nhow many stacks?", "the question follows the guidance unchanged")
	})

	t.Run("only the new question carries the style, earlier turns are untouched", func(t *testing.T) {
		client := &recordingClient{}
		history := []types.Message{
			{Role: types.RoleUser, Content: "how many stacks?"},
			{Role: types.RoleAssistant, Content: "289"},
		}

		result := NewExecutor(client, nil, &schema.AtmosConfiguration{}).Execute(t.Context(), Options{Prompt: "and in core?", History: history})

		require.True(t, result.Success)
		require.Len(t, client.historyPrompt, 3)
		assert.Equal(t, "how many stacks?", client.historyPrompt[0].Content)
		assert.Equal(t, "289", client.historyPrompt[1].Content)
		assert.Contains(t, client.historyPrompt[2].Content, "GitHub-flavored Markdown")
		assert.Contains(t, client.historyPrompt[2].Content, "and in core?")
		assert.Equal(t, "how many stacks?", history[0].Content, "the caller's history is not modified")
	})
}

func TestResponseStyle_ToolSystemPrompt(t *testing.T) {
	assert.Contains(t, toolSystemPrompt, "GitHub-flavored Markdown", "the tool path carries the style in its system prompt")
	assert.Contains(t, toolSystemPrompt, "Prefer specific tools over generic ones", "the tool guidance is kept")
}

func TestWithResponseStyle(t *testing.T) {
	got := withResponseStyle("hello")

	assert.True(t, len(got) > len("hello"))
	assert.Equal(t, "hello", got[len(got)-len("hello"):])
}

// systemPromptClient is a provider that can take a system prompt without tools.
type systemPromptClient struct {
	recordingClient
	gotSystemPrompt string
	gotMessages     []types.Message
}

func (c *systemPromptClient) SendMessageWithSystemPrompt(_ context.Context, systemPrompt string, messages []types.Message) (string, error) {
	c.gotSystemPrompt = systemPrompt
	c.gotMessages = append([]types.Message(nil), messages...)
	return "ok", nil
}

func TestResponseStyle_ProviderWithSystemPromptGetsItThere(t *testing.T) {
	client := &systemPromptClient{}
	history := []types.Message{
		{Role: types.RoleUser, Content: "how many stacks?"},
		{Role: types.RoleAssistant, Content: "289"},
	}

	result := NewExecutor(client, nil, &schema.AtmosConfiguration{}).Execute(t.Context(), Options{Prompt: "and in core?", History: history})

	require.True(t, result.Success)
	assert.Contains(t, client.gotSystemPrompt, "GitHub-flavored Markdown")
	require.Len(t, client.gotMessages, 3)
	assert.Equal(t, "and in core?", client.gotMessages[2].Content, "the question is sent unchanged")
	assert.Empty(t, client.plainPrompts, "the message-prefix fallback is not used")
	assert.Empty(t, client.historyPrompt, "the message-prefix fallback is not used")
}
