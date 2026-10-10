package interactive

import (
	"errors"
	"strings"

	"github.com/charmbracelet/huh"

	errUtils "github.com/cloudposse/atmos/errors"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/ai/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/pkg/terminal"
)

// isTTY reports whether a standard stream is a terminal. It is a seam: tests replace it to simulate
// terminals and pipes.
var isTTY = func(stream terminal.Stream) bool {
	return terminal.New().IsTTY(stream)
}

// isCI reports whether the process runs in a CI environment. It is a seam: tests replace it to simulate CI.
var isCI = telemetry.IsCI

// runForm runs a built huh form. It is a seam: tests replace it to drive the form in accessible
// mode with scripted input, so a prompt never blocks on a real terminal.
var runForm = func(form *huh.Form) error {
	return form.Run()
}

// TurnFunc runs one conversation turn and returns the answer text.
// The history argument holds every earlier question and answer, oldest first.
type TurnFunc func(question string, history []types.Message) (string, error)

// Conversation continues a one-shot question as a conversation: after each answer
// it asks whether there is a follow-up and runs another turn with the earlier turns as context.
type Conversation struct {
	// Enabled reports whether follow-up questions can be asked at all.
	Enabled func() bool
	// Prompt asks for the next question. An empty answer ends the conversation.
	Prompt func() (string, error)
}

// Continue offers follow-up questions after an answer, using terminal detection and a text prompt.
// Follow-ups are offered only when both stdin and stdout are terminals outside CI,
// so scripts and pipelines never wait for input.
func Continue(history []types.Message, turn TurnFunc) error {
	defer perf.Track(nil, "interactive.Continue")()

	return Conversation{Enabled: terminalAvailable, Prompt: promptFollowUp}.Continue(history, turn)
}

// Continue runs follow-up turns until the user finishes or a turn fails.
// Pressing ctrl+c or Esc at the prompt finishes the conversation without an error.
func (c Conversation) Continue(history []types.Message, turn TurnFunc) error {
	defer perf.Track(nil, "interactive.Conversation.Continue")()

	// Work on a copy: appending to the caller's slice could overwrite its spare capacity.
	history = append([]types.Message(nil), history...)

	for c.Enabled() {
		question, err := c.Prompt()
		if err != nil {
			if errors.Is(err, errUtils.ErrUserAborted) {
				return nil
			}
			return err
		}

		question = strings.TrimSpace(question)
		if question == "" {
			return nil
		}

		answer, err := turn(question, history)
		if err != nil {
			return err
		}

		history = append(
			history,
			types.Message{Role: types.RoleUser, Content: question},
			types.Message{Role: types.RoleAssistant, Content: answer},
		)
	}

	return nil
}

// terminalAvailable reports whether a person can type a follow-up and see the result.
func terminalAvailable() bool {
	return isTTY(terminal.Stdin) && isTTY(terminal.Stdout) && !isCI()
}

// promptFollowUp asks for the next question.
func promptFollowUp() (string, error) {
	var question string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Follow up?").
				Placeholder("Type a reply, or press Enter or Esc to finish").
				Value(&question),
		),
	).WithKeyMap(uiutils.NewAtmosKeyMap()).WithTheme(uiutils.NewAtmosHuhTheme())

	if err := runForm(form); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", errUtils.ErrUserAborted
		}
		return "", err
	}

	return question, nil
}
