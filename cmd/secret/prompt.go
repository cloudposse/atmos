package secret

import (
	"fmt"

	"github.com/charmbracelet/huh"

	errUtils "github.com/cloudposse/atmos/errors"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/perf"
)

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
	if err := form.Run(); err != nil {
		return "", fmt.Errorf("secret prompt failed: %w", err)
	}
	return value, nil
}

// confirmAction interactively asks the user to confirm a destructive action.
func confirmAction(title string) (bool, error) {
	defer perf.Track(nil, "secret.confirmAction")()

	var confirmed bool
	form := huh.NewForm(
		huh.NewGroup(huh.NewConfirm().Title(title).Value(&confirmed)),
	).WithTheme(uiutils.NewAtmosHuhTheme())
	if err := form.Run(); err != nil {
		return false, fmt.Errorf("confirmation prompt failed: %w", err)
	}
	return confirmed, nil
}
