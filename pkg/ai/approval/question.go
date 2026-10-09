package approval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"

	errUtils "github.com/cloudposse/atmos/errors"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	// AskUserQuestionTool is the name of Claude Code's tool for asking the user to choose.
	AskUserQuestionTool = "AskUserQuestion"

	// The answersKey constant is the tool input field that carries the user's answers back to the model.
	answersKey = "answers"

	// The otherLabel constant is the extra option that lets the user type an answer that is not listed.
	otherLabel = "Other…"

	// The multiAnswerSeparator constant joins the labels chosen in a multi-select question.
	multiAnswerSeparator = ", "

	noTerminalMessage = "The user cannot be asked questions here because no terminal is available. " +
		"Continue with reasonable assumptions, or say what you would need to know."
	malformedQuestionMessage = "The question could not be shown because its format was not understood."
)

// Question is one multiple-choice question from the model.
type Question struct {
	// Question is the full text of the question.
	Question string `json:"question"`
	// Header is a short label for the question, such as "Beverage".
	Header string `json:"header"`
	// Options are the choices offered.
	Options []QuestionOption `json:"options"`
	// MultiSelect allows choosing more than one option.
	MultiSelect bool `json:"multiSelect"`
}

// QuestionOption is one choice in a Question.
type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// Asker shows questions to the user and returns their answers keyed by question text.
type Asker interface {
	// Available reports whether the user can be asked at all (a terminal is attached).
	Available() bool
	// Ask shows each question in turn. Pressing ctrl+c returns errUtils.ErrUserAborted.
	Ask(ctx context.Context, questions []Question) (map[string]string, error)
}

// parseQuestions reads the questions out of an AskUserQuestion tool input.
func parseQuestions(input map[string]any) ([]Question, error) {
	raw, err := json.Marshal(input["questions"])
	if err != nil {
		return nil, err
	}

	var questions []Question
	if err := json.Unmarshal(raw, &questions); err != nil {
		return nil, err
	}

	valid := questions[:0]
	for _, q := range questions {
		if strings.TrimSpace(q.Question) != "" {
			valid = append(valid, q)
		}
	}
	return valid, nil
}

// askQuestions answers an AskUserQuestion request by asking the user.
// The answers go back to the model in the tool input, keyed by question text.
func (a *PermissionApprover) askQuestions(ctx context.Context, req Request) (Decision, error) {
	questions, err := parseQuestions(req.Input)
	if err != nil || len(questions) == 0 {
		return Decision{Message: malformedQuestionMessage}, nil
	}

	if !a.asker.Available() {
		return Decision{Message: noTerminalMessage}, nil
	}

	if a.before != nil {
		a.before()
	}
	if a.after != nil {
		defer a.after()
	}

	answers, err := a.asker.Ask(ctx, questions)
	if err != nil {
		if errors.Is(err, errUtils.ErrUserAborted) {
			return Decision{Interrupt: true, Message: abortedMessage}, nil
		}
		return Decision{}, fmt.Errorf("asking the user failed: %w", err)
	}

	updated := make(map[string]any, len(req.Input))
	for key, value := range req.Input {
		updated[key] = value
	}
	updated[answersKey] = answers

	return Decision{Allow: true, UpdatedInput: updated}, nil
}

// terminalAsker asks questions with interactive selection lists.
type terminalAsker struct{}

// Available reports whether stdin is a terminal.
func (terminalAsker) Available() bool {
	return terminal.New().IsTTY(terminal.Stdin)
}

// Ask shows each question as a selection list, with an "Other…" option for free text.
func (terminalAsker) Ask(_ context.Context, questions []Question) (map[string]string, error) {
	answers := make(map[string]string, len(questions))

	for _, q := range questions {
		answer, err := askOne(q)
		if err != nil {
			return nil, err
		}
		answers[q.Question] = answer
		ui.Success(fmt.Sprintf("%s: %s", questionLabel(q), ui.InlineCode(answer)))
	}
	ui.Writeln("")

	return answers, nil
}

// questionLabel is the short name shown in the receipt line.
func questionLabel(q Question) string {
	if strings.TrimSpace(q.Header) != "" {
		return q.Header
	}
	return q.Question
}

// askOne asks a single question and returns the answer text.
func askOne(q Question) (string, error) {
	chosen, err := choose(q)
	if err != nil {
		return "", err
	}

	answers := make([]string, 0, len(chosen))
	for _, label := range chosen {
		if label != otherLabel {
			answers = append(answers, label)
			continue
		}

		text, err := typeAnswer(q)
		if err != nil {
			return "", err
		}
		if text != "" {
			answers = append(answers, text)
		}
	}
	return strings.Join(answers, multiAnswerSeparator), nil
}

// choose shows the options and returns the labels the user picked.
func choose(q Question) ([]string, error) {
	// The size comes from model output, so it is not used in arithmetic; append grows the list for "Other…".
	options := make([]huh.Option[string], 0, len(q.Options))
	for _, opt := range q.Options {
		display := opt.Label
		if strings.TrimSpace(opt.Description) != "" {
			display += " — " + opt.Description
		}
		options = append(options, huh.NewOption(display, opt.Label))
	}
	options = append(options, huh.NewOption(otherLabel, otherLabel))

	if q.MultiSelect {
		var picked []string
		field := huh.NewMultiSelect[string]().Title(q.Question).Description(q.Header).Options(options...).Value(&picked)
		if err := runField(field); err != nil {
			return nil, err
		}
		return picked, nil
	}

	var single string
	field := huh.NewSelect[string]().Title(q.Question).Description(q.Header).Options(options...).Value(&single)
	if err := runField(field); err != nil {
		return nil, err
	}
	return []string{single}, nil
}

// typeAnswer asks for a free-text answer.
func typeAnswer(q Question) (string, error) {
	var text string
	field := huh.NewInput().Title(questionLabel(q)).Placeholder("Type your answer").Value(&text)
	if err := runField(field); err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}

// runField runs one field as its own form and maps a user abort to errUtils.ErrUserAborted.
func runField(field huh.Field) error {
	form := huh.NewForm(huh.NewGroup(field)).WithTheme(uiutils.NewAtmosHuhTheme())
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return errUtils.ErrUserAborted
		}
		return err
	}
	return nil
}
