package devcontainer

import (
	"fmt"
	"os"
	"sort"

	"github.com/charmbracelet/huh"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// listAvailableDevcontainers returns a sorted list of all available devcontainer names.
func listAvailableDevcontainers(atmosConfig *schema.AtmosConfiguration) ([]string, error) {
	if atmosConfig == nil || atmosConfig.Components.Devcontainer == nil {
		return nil, fmt.Errorf("%w: no devcontainers configured", errUtils.ErrDevcontainerNotFound)
	}

	var names []string
	for name := range atmosConfig.Components.Devcontainer {
		names = append(names, name)
	}

	sort.Strings(names)
	return names, nil
}

// promptForDevcontainer prompts the user to select a devcontainer from the available list.
func promptForDevcontainer(message string, devcontainers []string) (string, error) {
	if len(devcontainers) == 0 {
		return "", fmt.Errorf("%w: no devcontainers available", errUtils.ErrDevcontainerNotFound)
	}

	var selectedDevcontainer string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(message).
				Options(huh.NewOptions(devcontainers...)...).
				Value(&selectedDevcontainer),
		),
	)

	if err := form.Run(); err != nil {
		return "", fmt.Errorf("%w: failed to prompt for devcontainer: %v", errUtils.ErrUnsupportedInputType, err)
	}

	return selectedDevcontainer, nil
}

// getDevcontainerName gets the devcontainer name from args or prompts the user.
// If no name is provided in args and running interactively, prompts the user to select one.
func getDevcontainerName(args []string) (string, error) {
	// If name provided in args, use it.
	if len(args) > 0 && args[0] != "" {
		return args[0], nil
	}

	// Check if running in non-interactive mode (CI, piped, etc.).
	if !isatty.IsTerminal(os.Stdout.Fd()) {
		return "", fmt.Errorf("%w: devcontainer name is required in non-interactive mode", errUtils.ErrDevcontainerNameEmpty)
	}

	// Load atmos config to get available devcontainers.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return "", fmt.Errorf("failed to load atmos config: %w", err)
	}

	// Get list of available devcontainers.
	devcontainers, err := listAvailableDevcontainers(&atmosConfig)
	if err != nil {
		return "", err
	}

	if len(devcontainers) == 0 {
		return "", fmt.Errorf("%w: no devcontainers configured in atmos.yaml", errUtils.ErrDevcontainerNotFound)
	}

	// Prompt user to select a devcontainer.
	selectedName, err := promptForDevcontainer("Select a devcontainer:", devcontainers)
	if err != nil {
		return "", err
	}

	// Display selected devcontainer to stderr (so it doesn't interfere with stdout).
	fmt.Fprintf(os.Stderr, "\nSelected devcontainer: %s\n\n", selectedName)

	return selectedName, nil
}

// devcontainerNameCompletion provides autocomplete for devcontainer names.
func devcontainerNameCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// If we already have a devcontainer name argument, no more completions.
	if len(args) >= 1 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Load atmos config.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	// Get available devcontainers.
	devcontainers, err := listAvailableDevcontainers(&atmosConfig)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	return devcontainers, cobra.ShellCompDirectiveNoFileComp
}
