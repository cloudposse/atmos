package ui

import (
	"errors"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
)

// ConfirmApply prompts the user to confirm applying changes.
func ConfirmApply() (bool, error) {
	// Check TTY availability before showing prompt.
	if !term.IsTTYSupportForStdout() {
		return false, errUtils.ErrStreamingNotSupported
	}

	var confirm bool
	theme := uiutils.NewAtmosHuhTheme()

	prompt := huh.NewConfirm().
		Title("Do you want to apply these changes?").
		Affirmative("Yes").
		Negative("No").
		Value(&confirm).
		WithButtonAlignment(lipgloss.Left).
		WithTheme(theme)

	if err := prompt.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return false, errUtils.ErrUserAborted
		}
		return false, err
	}
	return confirm, nil
}

// ConfirmDestroy prompts the user to confirm destroying resources.
func ConfirmDestroy() (bool, error) {
	// Check TTY availability before showing prompt.
	if !term.IsTTYSupportForStdout() {
		return false, errUtils.ErrStreamingNotSupported
	}

	var confirm bool
	theme := uiutils.NewAtmosHuhTheme()

	prompt := huh.NewConfirm().
		Title("Do you want to destroy these resources?").
		Affirmative("Yes").
		Negative("No").
		Value(&confirm).
		WithButtonAlignment(lipgloss.Left).
		WithTheme(theme)

	if err := prompt.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return false, errUtils.ErrUserAborted
		}
		return false, err
	}
	return confirm, nil
}
