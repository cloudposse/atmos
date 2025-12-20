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

// FilterHandler prompts for fuzzy filter selection.
type FilterHandler struct {
	BaseHandler
}

func init() {
	Register(&FilterHandler{
		BaseHandler: NewBaseHandler("filter", CategoryInteractive, true),
	})
}

// Validate checks that the step has required fields.
func (h *FilterHandler) Validate(step *schema.WorkflowStep) error {
	if err := h.ValidateRequired(step, "prompt", step.Prompt); err != nil {
		return err
	}
	if len(step.Options) == 0 {
		return fmt.Errorf("step '%s' (filter): options is required", step.Name)
	}
	return nil
}

// Execute prompts for filtered selection and returns the chosen value(s).
func (h *FilterHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
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

	// Create custom keymap that adds ESC to quit keys.
	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit = key.NewBinding(
		key.WithKeys("ctrl+c", "esc"),
		key.WithHelp("ctrl+c/esc", "quit"),
	)

	// Check if multiple selection is allowed.
	limit := step.Limit
	if limit <= 0 {
		limit = 1 // Default to single selection.
	}

	if limit > 1 || step.Multiple {
		// Multi-select mode.
		var choices []string
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewMultiSelect[string]().
					Title(prompt).
					Description("Press ctrl+c or esc to cancel. Use space to select, enter to confirm.").
					Options(huh.NewOptions(options...)...).
					Limit(limit).
					Filterable(true).
					Value(&choices),
			),
		).WithKeyMap(keyMap).WithTheme(uiutils.NewAtmosHuhTheme())

		if err := form.Run(); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				return nil, errUtils.ErrUserAborted
			}
			return nil, fmt.Errorf("step '%s': filter selection failed: %w", step.Name, err)
		}

		// Return first value as primary, all values in Values.
		value := ""
		if len(choices) > 0 {
			value = choices[0]
		}
		return NewStepResult(value).WithValues(choices), nil
	}

	// Single-select mode with filtering.
	var choice string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(prompt).
				Description("Press ctrl+c or esc to cancel. Type to filter.").
				Options(huh.NewOptions(options...)...).
				Filtering(true).
				Value(&choice),
		),
	).WithKeyMap(keyMap).WithTheme(uiutils.NewAtmosHuhTheme())

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, errUtils.ErrUserAborted
		}
		return nil, fmt.Errorf("step '%s': filter selection failed: %w", step.Name, err)
	}

	return NewStepResult(choice), nil
}
