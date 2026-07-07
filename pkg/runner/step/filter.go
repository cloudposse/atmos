package step

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	defer perf.Track(nil, "step.FilterHandler.Validate")()

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
//
// When there is no TTY (e.g. in CI) and a `default` is configured, the default
// is returned without prompting. For multi-select (`multiple: true` or
// `limit > 1`) the default is treated as a comma-separated list. When there is
// no TTY and no `default` is set, resolveInteractive returns ErrStepTTYRequired.
func (h *FilterHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.FilterHandler.Execute")()

	useTTY, err := h.resolveInteractive(step)
	if err != nil {
		return nil, err
	}

	// Non-TTY with a configured default: use the default without prompting.
	if !useTTY {
		defaultVal, resolveErr := h.resolveDefault(step, vars)
		if resolveErr != nil {
			return nil, resolveErr
		}
		return h.resultFromDefault(step, defaultVal), nil
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
	if step.Multiple && limit <= 0 {
		// When multiple selection is enabled with no limit, allow selecting all options.
		limit = len(options)
	} else if limit <= 0 {
		// Default to single selection.
		limit = 1
	}

	if limit > 1 || step.Multiple {
		return h.executeMultiSelect(step.Name, prompt, options, limit)
	}

	return h.executeSingleSelect(step.Name, prompt, options)
}

// resolveDefault resolves template variables in the default value.
func (h *FilterHandler) resolveDefault(step *schema.WorkflowStep, vars *Variables) (string, error) {
	if step.Default == "" {
		return "", nil
	}
	defaultVal, err := vars.Resolve(step.Default)
	if err != nil {
		return "", fmt.Errorf("step '%s': failed to resolve default: %w", step.Name, err)
	}
	return defaultVal, nil
}

// resultFromDefault builds a StepResult from the configured default for the
// non-TTY path. For multi-select the default is split on commas into Values.
func (h *FilterHandler) resultFromDefault(step *schema.WorkflowStep, defaultVal string) *StepResult {
	if step.Multiple || step.Limit > 1 {
		values := splitFilterDefaults(defaultVal)
		first := ""
		if len(values) > 0 {
			first = values[0]
		}
		return NewStepResult(first).WithValues(values)
	}
	return NewStepResult(defaultVal)
}

// splitFilterDefaults splits a comma-separated default into trimmed,
// non-empty values for multi-select filter steps.
func splitFilterDefaults(defaultVal string) []string {
	if strings.TrimSpace(defaultVal) == "" {
		return nil
	}
	parts := strings.Split(defaultVal, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
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
