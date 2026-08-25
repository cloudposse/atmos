package cloudformation

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/huh"

	errUtils "github.com/cloudposse/atmos/errors"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
)

// confirmOperation prompts before a destructive/mutating operation unless
// --auto-approve was passed (deploy defaults it to true — see
// operationFlagOptions in cmd/aws/cloudformation). A seam for testing.
var confirmOperation = defaultConfirmOperation

// runConfirmField runs a huh field individually. Exposed as a package
// variable so tests can stub it — a real huh field requires an interactive
// TTY, which unit tests cannot reliably provide, and huh has no exported
// headless mode. Mirrors the runForm seam in pkg/auth/profile_fallback.go.
// Production code never reassigns this.
var runConfirmField = func(f huh.Field) error { return f.Run() }

// requireConfirmation prompts for the given operation unless auto-approve is set.
func requireConfirmation(operation Operation, stackName string, flags map[string]any) error {
	if operation != OperationApply && operation != OperationDelete {
		return nil
	}
	autoApprove, _ := flags["auto-approve"].(bool)
	if autoApprove {
		return nil
	}

	verb := "apply"
	if operation == OperationDelete {
		verb = "delete"
	}
	confirmed, err := confirmOperation(fmt.Sprintf("%s stack %q?", verb, stackName))
	if err != nil {
		return err
	}
	if !confirmed {
		return errUtils.ErrUserAborted
	}
	return nil
}

// defaultConfirmOperation shows a left-aligned Yes/No confirmation dialog.
func defaultConfirmOperation(message string) (bool, error) {
	confirm := false
	prompt := uiutils.NewAtmosConfirm().
		Title(message).
		Affirmative("Yes!").
		Negative("No.").
		Value(&confirm).
		WithTheme(uiutils.NewAtmosHuhTheme())
	if err := runConfirmField(prompt); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return false, fmt.Errorf("%w", errUtils.ErrUserAborted)
		}
		return false, err
	}
	return confirm, nil
}
