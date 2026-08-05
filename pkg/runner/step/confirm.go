package step

import (
	"context"
	"errors"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"

	errUtils "github.com/cloudposse/atmos/errors"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ConfirmHandler prompts for yes/no confirmation.
type ConfirmHandler struct {
	BaseHandler
}

func init() {
	Register(&ConfirmHandler{
		BaseHandler: NewBaseHandler("confirm", CategoryInteractive, true),
	})
}

// Validate checks that the step has required fields.
func (h *ConfirmHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.ConfirmHandler.Validate")()

	return h.ValidateRequired(step, "prompt", step.Prompt)
}

// Execute prompts for confirmation and returns "true" or "false".
//
// When there is no TTY (e.g. in CI) and a `default` is configured, the default
// ("yes"/"true" => "true", otherwise "false") is returned without prompting.
// When there is no TTY and no `default` is set, resolveInteractive returns
// ErrStepTTYRequired.
func (h *ConfirmHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.ConfirmHandler.Execute")()

	shouldPrompt, err := h.resolveInteractive(step)
	if err != nil {
		return nil, err
	}

	// Resolve templates in the default, then parse it — consistent with the
	// other interactive handlers (e.g. a default of "{{ .env.CONFIRM }}").
	defaultStr, err := h.ResolveDefault(ctx, step, vars)
	if err != nil {
		return nil, err
	}
	defaultVal := strings.ToLower(defaultStr) == "yes" || strings.ToLower(defaultStr) == "true"

	// Non-TTY with a configured default: use the default without prompting.
	if !shouldPrompt {
		return NewStepResult(confirmValue(defaultVal)), nil
	}

	prompt, err := h.ResolvePrompt(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	var confirmed bool

	// Create custom keymap that adds ESC to quit keys.
	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit = key.NewBinding(
		key.WithKeys("ctrl+c", "esc"),
		key.WithHelp("ctrl+c/esc", "quit"),
	)

	form := huh.NewForm(
		huh.NewGroup(
			uiutils.NewAtmosConfirm().
				Title(prompt).
				Affirmative("Yes").
				Negative("No").
				Value(&confirmed),
		),
	).WithKeyMap(keyMap).WithTheme(uiutils.NewAtmosHuhTheme())

	// Set initial value if default is true.
	if defaultVal {
		confirmed = true
	}

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, errUtils.ErrUserAborted
		}
		return nil, errUtils.Build(errUtils.ErrWorkflowStepFailed).
			WithCause(err).
			WithContext("step", step.Name).
			WithContext("type", step.Type).
			WithExplanation("Confirmation prompt failed").
			Err()
	}

	return NewStepResult(confirmValue(confirmed)), nil
}

// confirmValue maps a boolean confirmation to the step's string result.
func confirmValue(confirmed bool) string {
	if confirmed {
		return "true"
	}
	return "false"
}
