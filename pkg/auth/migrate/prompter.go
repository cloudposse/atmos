package migrate

import (
	"errors"

	"github.com/charmbracelet/huh"

	errUtils "github.com/cloudposse/atmos/errors"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/perf"
)

// HuhPrompter implements Prompter using charmbracelet/huh.
type HuhPrompter struct{}

// Confirm shows a yes/no prompt with the given title and returns the user's choice.
func (p *HuhPrompter) Confirm(title string) (bool, error) {
	defer perf.Track(nil, "migrate.HuhPrompter.Confirm")()

	var confirmed bool

	prompt := huh.NewConfirm().
		Title(title).
		Affirmative("Yes").
		Negative("No").
		Value(&confirmed).
		WithTheme(uiutils.NewAtmosHuhTheme())

	if err := prompt.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return false, errUtils.ErrUserAborted
		}

		return false, err
	}

	return confirmed, nil
}

// Select shows a list of options with the given title and returns the selected value.
func (p *HuhPrompter) Select(title string, options []string) (string, error) {
	defer perf.Track(nil, "migrate.HuhPrompter.Select")()

	var choice string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Options(huh.NewOptions(options...)...).
				Value(&choice),
		),
	).WithTheme(uiutils.NewAtmosHuhTheme())

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", errUtils.ErrUserAborted
		}

		return "", err
	}

	return choice, nil
}

// Input shows a text input prompt with the given title and default value.
func (p *HuhPrompter) Input(title, defaultValue string) (string, error) {
	defer perf.Track(nil, "migrate.HuhPrompter.Input")()

	value := defaultValue

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(title).
				Value(&value),
		),
	).WithTheme(uiutils.NewAtmosHuhTheme())

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", errUtils.ErrUserAborted
		}

		return "", err
	}

	return value, nil
}

// SelectAction shows the "apply all / step by step / cancel" prompt and returns the chosen action.
func (p *HuhPrompter) SelectAction() (Action, error) {
	defer perf.Track(nil, "migrate.HuhPrompter.SelectAction")()

	var choice string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("How would you like to proceed?").
				Options(
					huh.NewOption("Apply all changes", "apply-all"),
					huh.NewOption("Go step by step", "step-by-step"),
					huh.NewOption("Cancel", "cancel"),
				).
				Value(&choice),
		),
	).WithTheme(uiutils.NewAtmosHuhTheme())

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return ActionCancel, errUtils.ErrUserAborted
		}

		return ActionCancel, err
	}

	switch choice {
	case "apply-all":
		return ActionApplyAll, nil
	case "step-by-step":
		return ActionStepByStep, nil
	default:
		return ActionCancel, nil
	}
}
