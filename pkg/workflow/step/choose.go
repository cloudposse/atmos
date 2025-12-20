package step

import (
	"context"
	"errors"
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"

	errUtils "github.com/cloudposse/atmos/errors"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ChooseHandler prompts for single selection from a list.
type ChooseHandler struct {
	BaseHandler
}

func init() {
	Register(&ChooseHandler{
		BaseHandler: NewBaseHandler("choose", CategoryInteractive, true),
	})
}

// Validate checks that the step has required fields.
func (h *ChooseHandler) Validate(step *schema.WorkflowStep) error {
	if err := h.ValidateRequired(step, "prompt", step.Prompt); err != nil {
		return err
	}
	if len(step.Options) == 0 {
		return fmt.Errorf("step '%s' (choose): options is required", step.Name)
	}
	return nil
}

// Execute prompts for selection and returns the chosen value.
func (h *ChooseHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	if err := h.CheckTTY(step); err != nil {
		return nil, err
	}

	prompt, err := h.ResolvePrompt(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	// Resolve options (they might contain templates).
	options := make([]string, len(step.Options))
	for i, opt := range step.Options {
		resolved, err := vars.Resolve(opt)
		if err != nil {
			return nil, fmt.Errorf("step '%s': failed to resolve option %d: %w", step.Name, i, err)
		}
		options[i] = resolved
	}

	// Resolve default if present.
	defaultVal := step.Default
	if defaultVal != "" {
		defaultVal, err = vars.Resolve(defaultVal)
		if err != nil {
			return nil, fmt.Errorf("step '%s': failed to resolve default: %w", step.Name, err)
		}
	}

	var choice string

	// Find default option index if specified.
	huhOptions := huh.NewOptions(options...)
	for _, opt := range huhOptions {
		if opt.Value == defaultVal {
			opt.Selected(true)
			break
		}
	}

	// Create custom keymap that adds ESC to quit keys.
	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit = key.NewBinding(
		key.WithKeys("ctrl+c", "esc"),
		key.WithHelp("ctrl+c/esc", "quit"),
	)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(prompt).
				Description("Press ctrl+c or esc to cancel").
				Options(huhOptions...).
				Value(&choice),
		),
	).WithKeyMap(keyMap).WithTheme(uiutils.NewAtmosHuhTheme())

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, errUtils.ErrUserAborted
		}
		return nil, fmt.Errorf("step '%s': selection failed: %w", step.Name, err)
	}

	return NewStepResult(choice), nil
}
