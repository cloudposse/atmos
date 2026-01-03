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
	defer perf.Track(nil, "step.ChooseHandler.Validate")()

	if err := h.ValidateRequired(step, "prompt", step.Prompt); err != nil {
		return err
	}
	if len(step.Options) == 0 {
		return errUtils.Build(errUtils.ErrStepOptionsRequired).
			WithContext("step", step.Name).
			WithContext("type", "choose").
			Err()
	}
	return nil
}

// Execute prompts for selection and returns the chosen value.
func (h *ChooseHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.ChooseHandler.Execute")()

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

	defaultVal, err := h.resolveDefault(step, vars)
	if err != nil {
		return nil, err
	}

	return h.runSelectForm(step.Name, prompt, options, defaultVal)
}

// resolveOptions resolves template variables in options.
func (h *ChooseHandler) resolveOptions(step *schema.WorkflowStep, vars *Variables) ([]string, error) {
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

// resolveDefault resolves the default value if present.
func (h *ChooseHandler) resolveDefault(step *schema.WorkflowStep, vars *Variables) (string, error) {
	if step.Default == "" {
		return "", nil
	}
	defaultVal, err := vars.Resolve(step.Default)
	if err != nil {
		return "", fmt.Errorf("step '%s': failed to resolve default: %w", step.Name, err)
	}
	return defaultVal, nil
}

// createChooseKeyMap creates a keymap with ESC added to quit keys.
func (h *ChooseHandler) createChooseKeyMap() *huh.KeyMap {
	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit = key.NewBinding(
		key.WithKeys("ctrl+c", "esc"),
		key.WithHelp("ctrl+c/esc", "quit"),
	)
	return keyMap
}

// runSelectForm displays the select form and returns the result.
func (h *ChooseHandler) runSelectForm(stepName, prompt string, options []string, defaultVal string) (*StepResult, error) {
	var choice string
	huhOptions := huh.NewOptions(options...)
	for i := range huhOptions {
		if huhOptions[i].Value == defaultVal {
			huhOptions[i] = huhOptions[i].Selected(true)
			break
		}
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(prompt).
				Description("Press ctrl+c or esc to cancel").
				Options(huhOptions...).
				Value(&choice),
		),
	).WithKeyMap(h.createChooseKeyMap()).WithTheme(uiutils.NewAtmosHuhTheme())

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, errUtils.ErrUserAborted
		}
		return nil, fmt.Errorf("step '%s': selection failed: %w", stepName, err)
	}

	return NewStepResult(choice), nil
}
