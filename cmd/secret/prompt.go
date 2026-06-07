package secret

import (
	"fmt"

	"github.com/charmbracelet/huh"

	errUtils "github.com/cloudposse/atmos/errors"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/perf"
)

// runForm executes a built huh form. It is a seam: tests override it to run the form in accessible
// mode with scripted IO, so the real prompt bodies (titles, validators, error wrapping) are
// exercised without a live TTY. Production always calls form.Run().
var runForm = func(form *huh.Form) error { return form.Run() }

// promptForSecretValue interactively prompts for a secret value with masked input.
func promptForSecretValue() (string, error) {
	defer perf.Track(nil, "secret.promptForSecretValue")()

	var value string
	input := huh.NewInput().
		Title("Enter secret value").
		EchoMode(huh.EchoModePassword).
		Value(&value).
		Validate(func(s string) error {
			if s == "" {
				return errUtils.ErrMissingInput
			}
			return nil
		})

	form := huh.NewForm(huh.NewGroup(input)).WithTheme(uiutils.NewAtmosHuhTheme())
	if err := runForm(form); err != nil {
		return "", fmt.Errorf("secret prompt failed: %w", err)
	}
	return value, nil
}

// confirmAction interactively asks the user to confirm a destructive action.
func confirmAction(title string) (bool, error) {
	defer perf.Track(nil, "secret.confirmAction")()

	var confirmed bool
	form := huh.NewForm(
		huh.NewGroup(uiutils.NewAtmosConfirm().Title(title).Value(&confirmed)),
	).WithTheme(uiutils.NewAtmosHuhTheme())
	if err := runForm(form); err != nil {
		return false, fmt.Errorf("confirmation prompt failed: %w", err)
	}
	return confirmed, nil
}
