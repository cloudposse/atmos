package step

import (
	"context"
	"errors"
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"

	errUtils "github.com/cloudposse/atmos/errors"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// InputHandler prompts for single-line text input.
type InputHandler struct {
	BaseHandler
}

func init() {
	Register(&InputHandler{
		BaseHandler: NewBaseHandler("input", CategoryInteractive, true),
	})
}

// Validate checks that the step has required fields.
func (h *InputHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.InputHandler.Validate")()

	return h.ValidateRequired(step, "prompt", step.Prompt)
}

// Execute prompts for user input and returns the result.
func (h *InputHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.InputHandler.Execute")()

	if err := h.CheckTTY(step); err != nil {
		return nil, err
	}

	prompt, err := h.ResolvePrompt(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	placeholder, err := h.resolveOptionalValue(ctx, step, vars, step.Placeholder, "placeholder")
	if err != nil {
		return nil, err
	}

	defaultVal, err := h.resolveOptionalValue(ctx, step, vars, step.Default, "default")
	if err != nil {
		return nil, err
	}

	var value string

	input := huh.NewInput().
		Title(prompt).
		Placeholder(placeholder).
		Value(&value)

	// Set password mode if requested.
	if step.Password {
		input = input.EchoMode(huh.EchoModePassword)
	}

	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit = key.NewBinding(
		key.WithKeys("ctrl+c", "esc"),
		key.WithHelp("ctrl+c/esc", "quit"),
	)

	form := huh.NewForm(
		huh.NewGroup(input),
	).WithKeyMap(keyMap).WithTheme(uiutils.NewAtmosHuhTheme())

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, errUtils.ErrUserAborted
		}
		return nil, fmt.Errorf("step '%s': input failed: %w", step.Name, err)
	}

	// Use default if no value was entered.
	if value == "" && defaultVal != "" {
		value = defaultVal
	}

	return NewStepResult(value), nil
}

// resolveOptionalValue resolves a template string, returning empty if not set.
func (h *InputHandler) resolveOptionalValue(_ context.Context, step *schema.WorkflowStep, vars *Variables, val, field string) (string, error) {
	if val == "" {
		return "", nil
	}
	resolved, err := vars.Resolve(val)
	if err != nil {
		return "", fmt.Errorf("step '%s': failed to resolve %s: %w", step.Name, field, err)
	}
	return resolved, nil
}
