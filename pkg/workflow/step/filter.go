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
		return errUtils.Build(errUtils.ErrStepOptionsRequired).
			WithContext("step", step.Name).
			WithContext("type", "filter").
			Err()
	}
	return nil
}

// Execute prompts for filtered selection and returns the chosen value(s).
func (h *FilterHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.FilterHandler.Execute")()

	if err := h.CheckTTY(step); err != nil {
		return nil, err
	}

	prompt, err := h.ResolvePrompt(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	options, err := h.resolveOptions(step, vars)
	if err != nil {
		return nil, err
	}

	// Check if multiple selection is allowed.
	limit := step.Limit
	if limit <= 0 {
		limit = 1 // Default to single selection.
	}

	if limit > 1 || step.Multiple {
		return h.executeMultiSelect(step.Name, prompt, options, limit)
	}

	return h.executeSingleSelect(step.Name, prompt, options)
}

// resolveOptions resolves template variables in options.
func (h *FilterHandler) resolveOptions(step *schema.WorkflowStep, vars *Variables) ([]string, error) {
	options := make([]string, len(step.Options))
	for i, opt := range step.Options {
		resolved, err := vars.Resolve(opt)
		if err != nil {
			return nil, fmt.Errorf("step '%s': failed to resolve option %d: %w", step.Name, i, err)
		}
		options[i] = resolved
	}
	return options, nil
}

// createFilterKeyMap creates a keymap with ESC added to quit keys.
func (h *FilterHandler) createFilterKeyMap() *huh.KeyMap {
	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit = key.NewBinding(
		key.WithKeys("ctrl+c", "esc"),
		key.WithHelp("ctrl+c/esc", "quit"),
	)
	return keyMap
}

// executeMultiSelect runs multi-select mode with filtering.
func (h *FilterHandler) executeMultiSelect(stepName, prompt string, options []string, limit int) (*StepResult, error) {
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
	).WithKeyMap(h.createFilterKeyMap()).WithTheme(uiutils.NewAtmosHuhTheme())

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, errUtils.ErrUserAborted
		}
		return nil, fmt.Errorf("step '%s': filter selection failed: %w", stepName, err)
	}

	// Return first value as primary, all values in Values.
	value := ""
	if len(choices) > 0 {
		value = choices[0]
	}
	return NewStepResult(value).WithValues(choices), nil
}

// executeSingleSelect runs single-select mode with filtering.
func (h *FilterHandler) executeSingleSelect(stepName, prompt string, options []string) (*StepResult, error) {
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
	).WithKeyMap(h.createFilterKeyMap()).WithTheme(uiutils.NewAtmosHuhTheme())

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, errUtils.ErrUserAborted
		}
		return nil, fmt.Errorf("step '%s': filter selection failed: %w", stepName, err)
	}

	return NewStepResult(choice), nil
}
