package stack

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	iolib "github.com/cloudposse/atmos/pkg/io"
)

// runCreationForm allows tests to drive the real destination form with scripted input.
var runCreationForm = func(ctx context.Context, form *huh.Form) error {
	streams := iolib.GetContext()
	return form.WithInput(streams.Input()).WithOutput(streams.UI()).RunWithContext(ctx)
}

func creationDestination(ctx context.Context, v *viper.Viper) (string, string, error) {
	file, target := v.GetString("file"), v.GetString("target")
	if file != "" && target != "" {
		return "", "", fmt.Errorf("%w: --file and --target are mutually exclusive", errUtils.ErrInvalidFlag)
	}
	if file != "" || target != "" {
		return file, target, nil
	}
	if creationDefaults(v) {
		return "", "", fmt.Errorf("%w: --file or --target is required; pass a destination or run interactively", errUtils.ErrInvalidFlag)
	}
	return promptCreationDestination(ctx)
}

func promptCreationDestination(ctx context.Context) (string, string, error) {
	mode, destination := "file", ""
	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit = key.NewBinding(key.WithKeys("ctrl+c", "esc"), key.WithHelp("ctrl+c/esc", "quit"))
	form := huh.NewForm(
		huh.NewGroup(huh.NewSelect[string]().Title("Stack layout").
			Options(huh.NewOption("Single file", "file"), huh.NewOption("Directory", "target")).Value(&mode)),
		huh.NewGroup(huh.NewInput().Title("Destination").
			Description("Path relative to the project base path. Press ctrl+c or esc to cancel.").
			Value(&destination).Validate(func(value string) error {
			if strings.TrimSpace(value) == "" {
				return errUtils.ErrMissingInput
			}
			return nil
		})),
	).WithTheme(uiutils.NewAtmosHuhTheme()).WithKeyMap(keyMap)
	if err := runCreationForm(ctx, form); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", "", errUtils.ErrUserAborted
		}
		return "", "", fmt.Errorf("stack destination prompt failed: %w", err)
	}
	if mode == "target" {
		return "", destination, nil
	}
	return destination, "", nil
}
