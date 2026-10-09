package ai

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/executor"
	"github.com/cloudposse/atmos/pkg/ai/interactive"
	"github.com/cloudposse/atmos/pkg/ai/progress"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// scriptedAskClient answers with a fixed response and records what it was sent.
type scriptedAskClient struct {
	response string
	err      error

	sentPrompts []string
	sentHistory [][]types.Message
}

func (c *scriptedAskClient) SendMessage(_ context.Context, message string) (string, error) {
	c.sentPrompts = append(c.sentPrompts, message)
	c.sentHistory = append(c.sentHistory, nil)
	return c.response, c.err
}

func (c *scriptedAskClient) SendMessageWithHistory(_ context.Context, messages []types.Message) (string, error) {
	c.sentPrompts = append(c.sentPrompts, messages[len(messages)-1].Content)
	c.sentHistory = append(c.sentHistory, append([]types.Message(nil), messages[:len(messages)-1]...))
	return c.response, c.err
}

func (c *scriptedAskClient) SendMessageWithTools(context.Context, string, []tools.Tool) (*types.Response, error) {
	return nil, errors.New("not used")
}

func (c *scriptedAskClient) SendMessageWithToolsAndHistory(context.Context, []types.Message, []tools.Tool) (*types.Response, error) {
	return nil, errors.New("not used")
}

func (c *scriptedAskClient) SendMessageWithSystemPromptAndTools(context.Context, string, string, []types.Message, []tools.Tool) (*types.Response, error) {
	return nil, errors.New("not used")
}

func (c *scriptedAskClient) GetModel() string  { return "scripted" }
func (c *scriptedAskClient) GetMaxTokens() int { return 0 }

func newTestAskTurn(t *testing.T, client *scriptedAskClient) *askTurn {
	t.Helper()

	// Wire the markdown renderer used by data.Markdownf, mirroring root.go's PersistentPreRun.
	data.SetMarkdownRenderer(ui.Format)
	return &askTurn{
		exec:        executor.NewExecutor(client, nil, &schema.AtmosConfiguration{}),
		interactive: &interactive.Session{Progress: progress.New("Thinking…")},
		timeout:     time.Minute,
	}
}

func TestAskTurn_Run(t *testing.T) {
	t.Run("returns the answer text", func(t *testing.T) {
		client := &scriptedAskClient{response: "You have 289 stacks."}

		answer, err := newTestAskTurn(t, client).run("how many stacks?", "how many stacks?", nil)

		require.NoError(t, err)
		assert.Equal(t, "You have 289 stacks.", answer)
		assert.Equal(t, []string{"how many stacks?"}, client.sentPrompts)
	})

	t.Run("sends earlier turns as history so a follow-up has context", func(t *testing.T) {
		client := &scriptedAskClient{response: "Core has 40."}
		history := []types.Message{
			{Role: types.RoleUser, Content: "how many stacks?"},
			{Role: types.RoleAssistant, Content: "289"},
		}

		_, err := newTestAskTurn(t, client).run("and in core?", "and in core?", history)

		require.NoError(t, err)
		require.Len(t, client.sentHistory, 1)
		require.Len(t, client.sentHistory[0], 2)
		assert.Equal(t, "how many stacks?", client.sentHistory[0][0].Content)
		assert.Equal(t, "289", client.sentHistory[0][1].Content)
		assert.Equal(t, []string{"and in core?"}, client.sentPrompts)
	})

	t.Run("an empty answer is an error, not a blank line", func(t *testing.T) {
		client := &scriptedAskClient{response: "  \n"}

		answer, err := newTestAskTurn(t, client).run("hi", "hi", nil)

		require.ErrorIs(t, err, errUtils.ErrAIEmptyResponse)
		assert.Empty(t, answer)
	})

	t.Run("a provider failure is reported as an execution failure", func(t *testing.T) {
		client := &scriptedAskClient{err: errors.New("boom")}

		_, err := newTestAskTurn(t, client).run("hi", "hi", nil)

		require.ErrorIs(t, err, errUtils.ErrAIExecutionFailed)
		assert.NotErrorIs(t, err, errUtils.ErrUserAborted)
		assert.ErrorContains(t, err, "boom")
	})

	t.Run("a provider-managed timeout still produces a usable context", func(t *testing.T) {
		client := &scriptedAskClient{response: "ok"}
		turn := newTestAskTurn(t, client)
		turn.interactive.TimeoutManaged = true

		answer, err := turn.run("hi", "hi", nil)

		require.NoError(t, err)
		assert.Equal(t, "ok", answer)
	})
}
