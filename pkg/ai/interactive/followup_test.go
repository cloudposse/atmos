package interactive

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/charmbracelet/huh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/types"
	"github.com/cloudposse/atmos/pkg/terminal"
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

// overrideRunForm swaps the runForm seam for the duration of a test.
func overrideRunForm(t *testing.T, fn func(*huh.Form) error) {
	t.Helper()

	orig := runForm
	runForm = fn
	t.Cleanup(func() { runForm = orig })
}

// scriptFollowUps runs every follow-up form in huh's accessible mode, answering from script with one line
// per prompt. It returns what the forms printed and fails the test when the script is not fully used.
func scriptFollowUps(t *testing.T, script string) *bytes.Buffer {
	t.Helper()

	// Every accessible prompt builds its own line scanner, which would swallow the whole script on the
	// first prompt; handing the text out one byte at a time leaves the following lines for later prompts.
	in := iotest.OneByteReader(strings.NewReader(script))
	out := &bytes.Buffer{}
	overrideRunForm(t, func(form *huh.Form) error {
		return form.WithAccessible(true).WithInput(in).WithOutput(out).Run()
	})
	t.Cleanup(func() {
		rest, err := io.ReadAll(in)
		require.NoError(t, err)
		assert.Empty(t, string(rest), "the scripted answers must all be used")
	})
	return out
}

// overrideTerminalState replaces the terminal and CI detection seams for the duration of a test.
func overrideTerminalState(t *testing.T, stdin, stdout, ci bool) {
	t.Helper()

	origTTY, origCI := isTTY, isCI
	isTTY = func(stream terminal.Stream) bool {
		switch stream {
		case terminal.Stdin:
			return stdin
		case terminal.Stdout:
			return stdout
		default:
			return false
		}
	}
	isCI = func() bool { return ci }
	t.Cleanup(func() {
		isTTY, isCI = origTTY, origCI
	})
}

// TestTerminalAvailable covers each condition that must hold before a follow-up is offered.
func TestTerminalAvailable(t *testing.T) {
	tests := []struct {
		name   string
		stdin  bool
		stdout bool
		ci     bool
		want   bool
	}{
		{name: "terminals and no CI", stdin: true, stdout: true, ci: false, want: true},
		{name: "CI", stdin: true, stdout: true, ci: true, want: false},
		{name: "stdin is not a terminal", stdin: false, stdout: true, ci: false, want: false},
		{name: "stdout is not a terminal (piped result)", stdin: true, stdout: false, ci: false, want: false},
		{name: "nothing is a terminal", stdin: false, stdout: false, ci: false, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overrideTerminalState(t, tt.stdin, tt.stdout, tt.ci)

			assert.Equal(t, tt.want, terminalAvailable())
		})
	}
}

// TestPromptFollowUp covers what the follow-up prompt returns.
func TestPromptFollowUp(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   string
	}{
		{name: "typed text", script: "and in the core tenant?\n", want: "and in the core tenant?"},
		{name: "surrounding blanks are trimmed", script: "   by region   \n", want: "by region"},
		{name: "enter alone finishes with no text", script: "\n", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := scriptFollowUps(t, tt.script)

			got, err := promptFollowUp()

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Contains(t, out.String(), "Follow up?")
		})
	}
}

// TestPromptFollowUp_FormFailures covers abort and other failures of the follow-up form.
func TestPromptFollowUp_FormFailures(t *testing.T) {
	boom := errors.New("terminal exploded")

	t.Run("ctrl+c or Esc is the Atmos abort error", func(t *testing.T) {
		overrideRunForm(t, func(*huh.Form) error { return huh.ErrUserAborted })

		got, err := promptFollowUp()

		require.ErrorIs(t, err, errUtils.ErrUserAborted)
		assert.NotErrorIs(t, err, huh.ErrUserAborted)
		assert.Empty(t, got)
	})

	t.Run("other failures are returned unchanged", func(t *testing.T) {
		overrideRunForm(t, func(*huh.Form) error { return boom })

		got, err := promptFollowUp()

		require.ErrorIs(t, err, boom)
		assert.NotErrorIs(t, err, errUtils.ErrUserAborted, "only an abort maps to ErrUserAborted")
		assert.Empty(t, got)
	})
}

// TestContinue_UsesTerminalDetectionAndThePrompt runs the exported entry point end to end: terminal
// detection gates the conversation and the real prompt supplies the questions.
func TestContinue_UsesTerminalDetectionAndThePrompt(t *testing.T) {
	seed := []types.Message{
		{Role: types.RoleUser, Content: "how many stacks?"},
		{Role: types.RoleAssistant, Content: "289"},
	}

	t.Run("follow-ups run with the earlier turns as history", func(t *testing.T) {
		overrideTerminalState(t, true, true, false)
		scriptFollowUps(t, "and in core?\n\n")

		var questions []string
		var histories [][]types.Message
		err := Continue(seed, func(question string, history []types.Message) (string, error) {
			questions = append(questions, question)
			histories = append(histories, append([]types.Message(nil), history...))
			return "40", nil
		})

		require.NoError(t, err)
		assert.Equal(t, []string{"and in core?"}, questions, "an empty reply ends the conversation")
		require.Len(t, histories, 1)
		assert.Equal(t, seed, histories[0])
	})

	t.Run("no follow-up is offered without a terminal", func(t *testing.T) {
		overrideTerminalState(t, true, false, false)
		overrideRunForm(t, func(*huh.Form) error {
			t.Error("no form may be shown")
			return nil
		})

		err := Continue(seed, func(string, []types.Message) (string, error) {
			t.Error("no turn may run")
			return "", nil
		})

		require.NoError(t, err)
	})

	t.Run("no follow-up is offered in CI", func(t *testing.T) {
		overrideTerminalState(t, true, true, true)
		overrideRunForm(t, func(*huh.Form) error {
			t.Error("no form may be shown")
			return nil
		})

		err := Continue(seed, func(string, []types.Message) (string, error) {
			t.Error("no turn may run")
			return "", nil
		})

		require.NoError(t, err)
	})

	t.Run("pressing ctrl+c at the prompt ends the conversation without an error", func(t *testing.T) {
		overrideTerminalState(t, true, true, false)
		overrideRunForm(t, func(*huh.Form) error { return huh.ErrUserAborted })

		err := Continue(seed, func(string, []types.Message) (string, error) {
			t.Error("no turn may run")
			return "", nil
		})

		require.NoError(t, err)
	})
}
