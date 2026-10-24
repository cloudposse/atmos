package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ui/theme"
)

func newTestChatModelForTurnSteps(t *testing.T) *ChatModel {
	t.Helper()
	client := &mockAIClient{model: "test-model", maxTokens: 4096}
	model, err := NewChatModel(ChatModelParams{Client: client})
	require.NoError(t, err)
	return model
}

func TestFinishLastTurnStep(t *testing.T) {
	t.Run("no-op when there are no steps", func(t *testing.T) {
		model := newTestChatModelForTurnSteps(t)
		assert.NotPanics(t, func() { model.finishLastTurnStep(nil) })
		assert.Empty(t, model.turnSteps)
	})

	t.Run("marks the last step done on success", func(t *testing.T) {
		model := newTestChatModelForTurnSteps(t)
		model.turnSteps = []turnStep{{label: "step one", status: turnStepRunning, startedAt: time.Now()}}

		model.finishLastTurnStep(nil)

		last := model.turnSteps[len(model.turnSteps)-1]
		assert.Equal(t, turnStepDone, last.status)
		assert.Nil(t, last.err)
	})

	t.Run("marks the last step errored on failure", func(t *testing.T) {
		model := newTestChatModelForTurnSteps(t)
		model.turnSteps = []turnStep{{label: "step one", status: turnStepRunning, startedAt: time.Now()}}
		wantErr := errors.New("boom")

		model.finishLastTurnStep(wantErr)

		last := model.turnSteps[len(model.turnSteps)-1]
		assert.Equal(t, turnStepError, last.status)
		assert.Equal(t, wantErr, last.err)
	})

	t.Run("only touches the most recently started step", func(t *testing.T) {
		model := newTestChatModelForTurnSteps(t)
		model.turnSteps = []turnStep{
			{label: "first", status: turnStepRunning, startedAt: time.Now()},
			{label: "second", status: turnStepRunning, startedAt: time.Now()},
		}

		model.finishLastTurnStep(nil)

		assert.Equal(t, turnStepRunning, model.turnSteps[0].status)
		assert.Equal(t, turnStepDone, model.turnSteps[1].status)
	})
}

func TestHandleMessage_TurnStepMsgs(t *testing.T) {
	model := newTestChatModelForTurnSteps(t)
	cmds := []tea.Cmd{}

	handled, _ := model.handleMessage(turnStepStartedMsg{kind: turnStepKindTool, label: "list_stacks", status: turnStepRunning, startedAt: time.Now()}, &cmds)
	assert.True(t, handled)
	require.Len(t, model.turnSteps, 1)
	assert.Equal(t, "list_stacks", model.turnSteps[0].label)
	assert.Equal(t, turnStepRunning, model.turnSteps[0].status)

	handled, _ = model.handleMessage(turnStepFinishedMsg{err: nil}, &cmds)
	assert.True(t, handled)
	assert.Equal(t, turnStepDone, model.turnSteps[0].status)
}

func TestLoadingFooterContent_TurnSteps(t *testing.T) {
	t.Run("falls back to loadingText when there are no turn steps", func(t *testing.T) {
		model := newTestChatModelForTurnSteps(t)
		model.loadingText = "AI is thinking..."

		content := model.loadingFooterContent()

		assert.Contains(t, content, "AI is thinking...")
	})

	t.Run("renders a checklist when turn steps are present", func(t *testing.T) {
		model := newTestChatModelForTurnSteps(t)
		model.turnSteps = []turnStep{
			{label: "list_stacks", status: turnStepDone, duration: 2 * time.Second},
			{label: "describe_component", status: turnStepRunning, startedAt: time.Now()},
		}

		content := model.loadingFooterContent()

		assert.Contains(t, content, "list_stacks")
		assert.Contains(t, content, "describe_component")
	})

	t.Run("renders an error suffix for a failed step", func(t *testing.T) {
		model := newTestChatModelForTurnSteps(t)
		model.turnSteps = []turnStep{
			{label: "list_stacks", status: turnStepError, err: errors.New("boom")},
		}

		content := model.loadingFooterContent()

		assert.Contains(t, content, "list_stacks")
		assert.Contains(t, content, "boom")
	})

	t.Run("caps rendered steps to the footer budget", func(t *testing.T) {
		model := newTestChatModelForTurnSteps(t)
		for i := 0; i < maxDisplayedTurnSteps+5; i++ {
			model.turnSteps = append(model.turnSteps, turnStep{label: "step", status: turnStepDone})
		}

		content := model.loadingFooterContent()
		lines := strings.Split(content, newlineChar)

		assert.LessOrEqual(t, len(lines), maxDisplayedTurnSteps+1) // +1 for the "N earlier step(s) omitted" line.
		assert.Contains(t, content, "earlier step")
	})
}

func TestInitSpinner_UsesThemeStyle(t *testing.T) {
	s := initSpinner()
	assert.Equal(t, theme.GetCurrentStyles().Spinner, s.Style)
}
