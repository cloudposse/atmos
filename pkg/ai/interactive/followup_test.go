package interactive

import (
	"errors"
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/types"
)

var errBoom = errors.New("boom")

// scriptedPrompt returns a prompt that yields the given answers in order, then an error if asked again.
func scriptedPrompt(t *testing.T, answers ...any) func() (string, error) {
	t.Helper()
	next := 0
	return func() (string, error) {
		require.Less(t, next, len(answers), "prompted more often than the test scripted")
		answer := answers[next]
		next++
		if err, ok := answer.(error); ok {
			return "", err
		}
		return answer.(string), nil
	}
}

func always() bool { return true }

func TestConversation_Continue(t *testing.T) {
	seed := []types.Message{
		{Role: types.RoleUser, Content: "how many stacks?"},
		{Role: types.RoleAssistant, Content: "289"},
	}

	t.Run("runs follow-ups with the earlier turns as history until an empty answer", func(t *testing.T) {
		var seen [][]types.Message
		var questions []string
		turn := func(question string, history []types.Message) (string, error) {
			questions = append(questions, question)
			seen = append(seen, append([]types.Message(nil), history...))
			return "answer to " + question, nil
		}

		conv := Conversation{Enabled: always, Prompt: scriptedPrompt(t, "break it down by tenant", "  and by region ", "")}
		require.NoError(t, conv.Continue(seed, turn))

		assert.Equal(t, []string{"break it down by tenant", "and by region"}, questions, "questions are trimmed")
		require.Len(t, seen, 2)
		assert.Len(t, seen[0], 2)
		require.Len(t, seen[1], 4)
		assert.Equal(t, "how many stacks?", seen[1][0].Content, "first element is the original question")
		assert.Equal(t, "answer to break it down by tenant", seen[1][3].Content, "last element is the previous answer")
		assert.Equal(t, types.RoleAssistant, seen[1][3].Role)
		assert.Equal(t, types.RoleUser, seen[1][2].Role)
	})

	t.Run("does not prompt when follow-ups are disabled", func(t *testing.T) {
		conv := Conversation{
			Enabled: func() bool { return false },
			Prompt:  func() (string, error) { t.Fatal("must not prompt"); return "", nil },
		}
		turn := func(string, []types.Message) (string, error) { t.Fatal("must not run a turn"); return "", nil }

		require.NoError(t, conv.Continue(seed, turn))
	})

	t.Run("ctrl+c at the prompt finishes without an error", func(t *testing.T) {
		conv := Conversation{Enabled: always, Prompt: scriptedPrompt(t, fmt.Errorf("prompt: %w", errUtils.ErrUserAborted))}
		turn := func(string, []types.Message) (string, error) { t.Fatal("must not run a turn"); return "", nil }

		require.NoError(t, conv.Continue(seed, turn))
	})

	t.Run("a prompt failure is returned", func(t *testing.T) {
		conv := Conversation{Enabled: always, Prompt: scriptedPrompt(t, errBoom)}

		err := conv.Continue(seed, func(string, []types.Message) (string, error) { return "", nil })
		require.ErrorIs(t, err, errBoom)
	})

	t.Run("a failing turn stops the conversation with its error", func(t *testing.T) {
		conv := Conversation{Enabled: always, Prompt: scriptedPrompt(t, "again")}

		err := conv.Continue(seed, func(string, []types.Message) (string, error) { return "", errBoom })
		require.ErrorIs(t, err, errBoom)
	})

	t.Run("does not modify the caller's history", func(t *testing.T) {
		history := make([]types.Message, 2, 8)
		copy(history, seed)
		conv := Conversation{Enabled: always, Prompt: scriptedPrompt(t, "more", "")}

		require.NoError(t, conv.Continue(history, func(string, []types.Message) (string, error) { return "ok", nil }))

		assert.Equal(t, seed, history[:2])
		assert.Equal(t, types.Message{}, history[:cap(history)][2], "spare capacity of the caller's slice is untouched")
	})
}

func TestTerminalAvailable_FalseWithoutTerminals(t *testing.T) {
	// Tests run without a terminal on stdin, so follow-ups must be off; this is what keeps CI and pipes safe.
	assert.False(t, terminalAvailable())
}

func TestTerminalAvailable_ForcedOutputDoesNotEnableInput(t *testing.T) {
	previous := viper.Get("force-tty")
	viper.Set("force-tty", true)
	t.Cleanup(func() { viper.Set("force-tty", previous) })
	assert.False(t, terminalAvailable())
}

func TestFollowUpForm_EscapeAborts(t *testing.T) {
	var question string
	form := newFollowUpForm(&question)
	_, _ = form.Update(tea.KeyMsg{Type: tea.KeyEsc})
	assert.Equal(t, huh.StateAborted, form.State)
	assert.Empty(t, question)
}
