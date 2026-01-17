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
func (h *ConfirmHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.ConfirmHandler.Execute")()

	if err := h.CheckTTY(step); err != nil {
		return nil, err
	}

	prompt, err := h.ResolvePrompt(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	// Parse default value.
	defaultVal := strings.ToLower(step.Default) == "yes" || strings.ToLower(step.Default) == "true"

	var confirmed bool

	// Create custom keymap that adds ESC to quit keys.
	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit = key.NewBinding(
		key.WithKeys("ctrl+c", "esc"),
		key.WithHelp("ctrl+c/esc", "quit"),
	)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
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

	value := "false"
	if confirmed {
		value = "true"
	}

	return NewStepResult(value), nil
}
