package store

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

// validateStoreValue rejects an empty prompted value. Extracted from promptForStoreValue so its
// logic is unit-testable on every platform without going through huh's form/PTY machinery, which
// TestPromptForStoreValue_PTY needs a real TTY for and skips on Windows.
func validateStoreValue(s string) error {
	if s == "" {
		return errUtils.ErrMissingInput
	}
	return nil
}

// promptForStoreValue interactively prompts for a value with masked input -- a store can back a
// `secret: true` backend, so input is masked the same way `atmos secret set` masks it.
func promptForStoreValue() (string, error) {
	defer perf.Track(nil, "store.promptForStoreValue")()

	var value string
	input := huh.NewInput().
		Title("Enter value").
		EchoMode(huh.EchoModePassword).
		Value(&value).
		Validate(validateStoreValue)

	form := huh.NewForm(huh.NewGroup(input)).WithTheme(uiutils.NewAtmosHuhTheme())
	if err := runForm(form); err != nil {
		return "", fmt.Errorf("store prompt failed: %w", err)
	}
	return value, nil
}

// confirmAction interactively asks the user to confirm a destructive action.
func confirmAction(title string) (bool, error) {
	defer perf.Track(nil, "store.confirmAction")()

	var confirmed bool
	form := huh.NewForm(
		huh.NewGroup(uiutils.NewAtmosConfirm().Title(title).Value(&confirmed)),
	).WithTheme(uiutils.NewAtmosHuhTheme())
	if err := runForm(form); err != nil {
		return false, fmt.Errorf("confirmation prompt failed: %w", err)
	}
	return confirmed, nil
}
