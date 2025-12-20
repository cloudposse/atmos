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

const defaultWriteHeight = 5

// WriteHandler prompts for multi-line text input.
type WriteHandler struct {
	BaseHandler
}

func init() {
	Register(&WriteHandler{
		BaseHandler: NewBaseHandler("write", CategoryInteractive, true),
	})
}

// Validate checks that the step has required fields.
func (h *WriteHandler) Validate(step *schema.WorkflowStep) error {
	return h.ValidateRequired(step, "prompt", step.Prompt)
}

// Execute prompts for multi-line input and returns the result.
func (h *WriteHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	if err := h.CheckTTY(step); err != nil {
		return nil, err
	}

	prompt, err := h.ResolvePrompt(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	// Resolve placeholder if present.
	placeholder := step.Placeholder
	if placeholder != "" {
		placeholder, err = vars.Resolve(placeholder)
		if err != nil {
			return nil, fmt.Errorf("step '%s': failed to resolve placeholder: %w", step.Name, err)
		}
	}

	var value string

	// Create custom keymap that adds ESC to quit keys.
	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit = key.NewBinding(
		key.WithKeys("ctrl+c", "esc"),
		key.WithHelp("ctrl+c/esc", "quit"),
	)

	text := huh.NewText().
		Title(prompt).
		Placeholder(placeholder).
		Value(&value)

	// Set editor height if specified.
	if step.Height > 0 {
		text = text.Lines(step.Height)
	} else {
		text = text.Lines(defaultWriteHeight)
	}

	form := huh.NewForm(
		huh.NewGroup(text),
	).WithKeyMap(keyMap).WithTheme(uiutils.NewAtmosHuhTheme())

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, errUtils.ErrUserAborted
		}
		return nil, fmt.Errorf("step '%s': text input failed: %w", step.Name, err)
	}

	return NewStepResult(value), nil
}
