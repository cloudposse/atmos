package ai

import (
	"context"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/term"

	errUtils "github.com/cloudposse/atmos/errors"
	aiclient "github.com/cloudposse/atmos/pkg/ai"
	"github.com/cloudposse/atmos/pkg/ai/executor"
	"github.com/cloudposse/atmos/pkg/ai/interactive"
	"github.com/cloudposse/atmos/pkg/ai/progress"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
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

	return newAskTurnFor(t, client)
}

// newAskTurnFor builds an askTurn around any AI client.
func newAskTurnFor(t *testing.T, client aiclient.Client) *askTurn {
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
		require.Len(t, client.sentPrompts, 1)
		assert.True(t, strings.HasSuffix(client.sentPrompts[0], "how many stacks?"), "the question is sent last")
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
		require.Len(t, client.sentPrompts, 1)
		assert.True(t, strings.HasSuffix(client.sentPrompts[0], "and in core?"), "the follow-up is sent last")
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

// waitingAskClient waits in SendMessage until its context ends, then fails with the context's error.
// The onWait function, when set, runs repeatedly while waiting (for example to press a key).
type waitingAskClient struct {
	scriptedAskClient

	onWait func()

	mu      sync.Mutex
	ctxErr  error
	waitFor time.Duration
}

// SendMessage waits for the context to end and reports why it ended.
func (c *waitingAskClient) SendMessage(ctx context.Context, _ string) (string, error) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	giveUp := time.After(c.waitFor)

	for {
		select {
		case <-ctx.Done():
			c.mu.Lock()
			c.ctxErr = ctx.Err()
			c.mu.Unlock()
			return "", ctx.Err()
		case <-ticker.C:
			if c.onWait != nil {
				c.onWait()
			}
		case <-giveUp:
			return "", errors.New("the context was never canceled")
		}
	}
}

// contextError returns why the context ended, as seen by the client.
func (c *waitingAskClient) contextError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ctxErr
}

// lockedWriter collects what the spinner draws; the renderer writes from its own goroutine.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

// Write forwards p to the wrapped writer.
func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// TestAskTurn_Run_CtrlCWhileWaitingAbortsTheTurn presses ctrl+c on a real (pseudo) terminal while the
// spinner shows. The terminal is in raw mode, so the key is not a signal: the spinner reports it and the
// turn must end as a user abort, with the provider's context canceled.
func TestAskTurn_Run_CtrlCWhileWaitingAbortsTheTurn(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty is not supported on Windows")
	}
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Skipf("pty unavailable in this environment: %v", err)
	}
	t.Cleanup(func() {
		_ = ptmx.Close()
		_ = tty.Close()
	})
	// Raw mode up front: a ctrl+c byte reaches the spinner as a key instead of being handled by the line discipline.
	_, err = term.MakeRaw(int(tty.Fd()))
	require.NoError(t, err)

	// Terminal output is forced on; the spinner reads from the pseudo terminal and draws to a sink.
	viper.Set("force-tty", true)
	t.Cleanup(func() { viper.Set("force-tty", false) })
	origUI := iolib.UI
	iolib.UI = &lockedWriter{w: io.Discard}
	t.Cleanup(func() { iolib.UI = origUI })
	origStdin := os.Stdin
	os.Stdin = tty
	t.Cleanup(func() { os.Stdin = origStdin })

	// The key is sent repeatedly until the turn is canceled, because the spinner starts reading
	// input a moment after the request begins.
	client := &waitingAskClient{waitFor: 30 * time.Second}
	client.onWait = func() { _, _ = ptmx.Write([]byte{0x03}) }
	turn := newAskTurnFor(t, client)

	answer, err := turn.run("how many stacks?", "how many stacks?", nil)

	require.ErrorIs(t, err, errUtils.ErrUserAborted)
	assert.NotErrorIs(t, err, errUtils.ErrAIExecutionFailed, "an abort is not reported as an execution failure")
	assert.Empty(t, answer)
	assert.ErrorIs(t, client.contextError(), context.Canceled, "the provider's request was canceled")
}

// TestAskTurn_Run_TimeoutIsNotAUserAbort is the negative case of the ctrl+c recovery: a deadline that
// expires also ends the request, but it must be reported as a failure, not as the user's abort.
func TestAskTurn_Run_TimeoutIsNotAUserAbort(t *testing.T) {
	client := &waitingAskClient{waitFor: 30 * time.Second}
	turn := newAskTurnFor(t, client)
	turn.timeout = 50 * time.Millisecond

	answer, err := turn.run("how many stacks?", "how many stacks?", nil)

	require.Error(t, err)
	assert.NotErrorIs(t, err, errUtils.ErrUserAborted)
	assert.Empty(t, answer)
	assert.ErrorIs(t, client.contextError(), context.DeadlineExceeded)
}

// TestAskTurn_Run_RenderFailureIsReturned covers a missing markdown renderer: the failure is reported, and
// no answer is handed back as if it had been shown.
func TestAskTurn_Run_RenderFailureIsReturned(t *testing.T) {
	client := &scriptedAskClient{response: "You have 289 stacks."}
	turn := newAskTurnFor(t, client)
	data.SetMarkdownRenderer(nil)
	t.Cleanup(func() { data.SetMarkdownRenderer(ui.Format) })

	answer, err := turn.run("how many stacks?", "how many stacks?", nil)

	require.ErrorIs(t, err, errUtils.ErrUIFormatterNotInitialized)
	assert.Empty(t, answer)
}
