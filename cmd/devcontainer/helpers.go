package devcontainer

import (
	"fmt"
	"os"
	"sort"

	"github.com/charmbracelet/huh"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

// listAvailableDevcontainers returns a sorted list of devcontainer names from the provided AtmosConfiguration.
// If atmosConfig is nil or its Devcontainer field is nil, it returns an error wrapped with ErrDevcontainerNotFound.
func listAvailableDevcontainers(atmosConfig *schema.AtmosConfiguration) ([]string, error) {
	if atmosConfig == nil || atmosConfig.Devcontainer == nil {
		return nil, fmt.Errorf("%w: no devcontainers configured", errUtils.ErrDevcontainerNotFound)
	}

	var names []string
	for name := range atmosConfig.Devcontainer {
		names = append(names, name)
	}

	sort.Strings(names)
	return names, nil
}

// promptForDevcontainer prompts the user to select a devcontainer from the provided list.
// Returns the selected devcontainer name.
// Returns an error if the devcontainer list is empty or if the interactive prompt fails.
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
		return "", fmt.Errorf("failed to prompt for devcontainer: %w", err)
	}

	return selectedDevcontainer, nil
}

// getDevcontainerName gets the devcontainer name from args or prompts the user.
// getDevcontainerName obtains a devcontainer name from the provided arguments or, when none is given,
// prompts the user to select one in an interactive terminal.
//
// If the first element of args is non-empty, that value is returned. If no name is provided and the
// process is not running in an interactive terminal, an error is returned indicating the name is
// required. When running interactively, the function loads the Atmos configuration to discover
// available devcontainers, prompts the user to choose one, prints the chosen name to stderr, and
// returns it.
//
// args is the command-line arguments slice; its first element, if present and non-empty, is used as
// the devcontainer name.
//
// Returns the selected devcontainer name, or an error if a name could not be determined or on any
// failure during configuration loading or prompting.
func getDevcontainerName(args []string) (string, error) {
	// If name provided in args, use it.
	if len(args) > 0 && args[0] != "" {
		return args[0], nil
	}

	// Check if running in non-interactive mode (CI, piped, etc.).
	// Check stdin since prompts read from stdin, not stdout.
	if !isatty.IsTerminal(os.Stdin.Fd()) && !isatty.IsCygwinTerminal(os.Stdin.Fd()) {
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

// isAuthConfigured reports whether authentication is configured for the provided AuthConfig.
// It returns true if authConfig is non-nil and contains at least one identity, false otherwise.
func isAuthConfigured(authConfig *schema.AuthConfig) bool {
	return authConfig != nil && len(authConfig.Identities) > 0
}

// createUnauthenticatedAuthManager creates an auth manager without authenticating.
// createUnauthenticatedAuthManager creates an AuthManager initialized with an empty AuthContext so callers can access identities (for example, GetDefaultIdentity) without performing authentication.
// It returns the configured AuthManager, or an error if the manager could not be initialized.
func createUnauthenticatedAuthManager(authConfig *schema.AuthConfig) (auth.AuthManager, error) {
	authStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{},
	}

	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()
	authManager, err := auth.NewAuthManager(authConfig, credStore, validator, authStackInfo)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrFailedToInitializeAuthManager).
			WithExplanation("Failed to create authentication manager").
			WithContext("error", err.Error()).
			Err()
	}

	return authManager, nil
}

// initCommandWithFlags initializes a command's flags using StandardParser.
// initCommandWithFlags registers command flags with the provided parser and binds them to Viper.
// It panics if binding the parser to Viper fails.
func initCommandWithFlags(cmd *cobra.Command, parser *flags.StandardParser) {
	// Register flags using the standard RegisterFlags method.
	parser.RegisterFlags(cmd)

	// Bind flags to Viper for environment variable support.
	if err := parser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}