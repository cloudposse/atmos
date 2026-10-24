package secret

import (
	"fmt"

	"github.com/charmbracelet/huh"

	errUtils "github.com/cloudposse/atmos/errors"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/flags"
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

// interactiveFn reports whether prompts can be shown (a TTY, interactive mode enabled, and not CI).
// It is a seam so tests can exercise the non-interactive path without a real terminal.
var interactiveFn = flags.IsInteractive

// confirmActionInteractive asks for confirmation only when a prompt can actually be shown. Without
// a TTY (pipelines, CI, scripts) the underlying form would fail with an opaque terminal error
// ("could not open a new TTY"), so it fails with a clear error and a hint to pass --force instead.
func confirmActionInteractive(title string) (bool, error) {
	defer perf.Track(nil, "secret.confirmActionInteractive")()

	if !interactiveFn() {
		return false, errUtils.Build(errUtils.ErrInteractiveModeNotAvailable).
			WithExplanationf("Confirmation is required (%s) but no interactive terminal is available.", title).
			WithHint("Pass `--force` to proceed without confirmation").
			Err()
	}
	return confirmAction(title)
}
