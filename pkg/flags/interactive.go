package flags

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/telemetry"
)

// isInteractive checks if interactive prompts should be shown.
// Interactive mode requires:
// 1. --interactive flag is true (or ATMOS_INTERACTIVE env var).
// 2. Stdin is a TTY (for user input).
// 3. Not running in CI environment.
//
// This ensures prompts only appear in truly interactive contexts and gracefully
// degrade to standard errors in pipelines, scripts, and CI environments.
func isInteractive() bool {
	defer perf.Track(nil, "flags.isInteractive")()

	// Check if interactive mode is enabled via flag or environment.
	if !viper.GetBool("interactive") {
		return false
	}

	// Check if stdin is a TTY and not in CI.
	return term.IsTTYSupportForStdin() && !telemetry.IsCI()
}

// PromptForValue shows an interactive Huh selector with the given options.
// Returns the selected value or an error.
//
// This is the core prompting function used by all three use cases:
// 1. Missing required flags.
// 2. Optional value flags (sentinel pattern).
// 3. Missing required positional arguments.
func PromptForValue(name, title string, options []string) (string, error) {
	defer perf.Track(nil, "flags.PromptForValue")()

	if !isInteractive() {
		return "", errUtils.ErrInteractiveModeNotAvailable
	}

	if len(options) == 0 {
		return "", fmt.Errorf("%w: %s", errUtils.ErrNoOptionsAvailable, name)
	}

	var choice string

	// Create Huh selector with Atmos theme.
	// Limit height to 20 rows to prevent excessive scrolling and reduce terminal rendering artifacts.
	// Note: Huh v0.8.0 has case-sensitive filtering (by design).
	// Users can filter by typing "/" followed by search text, but it only matches exact case.
	// Example: typing "dark" matches "neobones_dark" but not "Builtin Dark"
	// TODO: Consider filing upstream feature request for case-insensitive filtering option.
	selector := huh.NewSelect[string]().
		Value(&choice).
		Options(huh.NewOptions(options...)...).
		Title(title).
		Height(20).
		WithTheme(uiutils.NewAtmosHuhTheme())

	// Run selector.
	if err := selector.Run(); err != nil {
		return "", fmt.Errorf("prompt failed: %w", err)
	}

	return choice, nil
}

// PromptForMissingRequired prompts for a required flag that is missing.
// This is Use Case 1: Missing Required Flags
//
// Example:
//
//	$ atmos describe component vpc
//	? Choose a stack
//	  ue2-dev
//	> ue2-prod
func PromptForMissingRequired(flagName, promptTitle string, completionFunc CompletionFunc, cmd *cobra.Command, args []string) (string, error) {
	defer perf.Track(nil, "flags.PromptForMissingRequired")()

	if !isInteractive() {
		return "", nil // Gracefully return empty - Cobra will handle the error
	}

	// Call completion function to get options.
	options, _ := completionFunc(cmd, args, "")
	if len(options) == 0 {
		return "", nil // No options available, let Cobra handle the error
	}

	return PromptForValue(flagName, promptTitle, options)
}

// PromptForOptionalValue prompts for a flag that was used without a value.
// This is Use Case 2: Optional Value Flags (like --identity pattern).
//
// The flag must have NoOptDefVal set to cfg.IdentityFlagSelectValue ("__SELECT__").
// When user provides --flag without value, Cobra sets it to the sentinel value,
// and we detect this to show the prompt.
//
// Example:
//
//	$ atmos list stacks --format
//	? Choose output format
//	  yaml
//	> json
//	  table
//
//nolint:revive // argument-limit: 6 args needed for prompt context
func PromptForOptionalValue(flagName, flagValue, promptTitle string, completionFunc CompletionFunc, cmd *cobra.Command, args []string) (string, error) {
	defer perf.Track(nil, "flags.PromptForOptionalValue")()

	// Check if flag value matches the sentinel (indicating user wants interactive selection).
	if flagValue != cfg.IdentityFlagSelectValue {
		return flagValue, nil // Real value provided, no prompt needed
	}

	if !isInteractive() {
		return "", nil // Gracefully return empty - command can use default
	}

	// Call completion function to get options.
	options, _ := completionFunc(cmd, args, "")
	if len(options) == 0 {
		return "", nil // No options available, use default
	}

	return PromptForValue(flagName, promptTitle, options)
}

// PromptForPositionalArg prompts for a required positional argument that is missing.
// This is Use Case 3: Missing Required Positional Arguments
//
// Example:
//
//	$ atmos theme show
//	? Choose a theme to preview
//	  Dracula
//	  Tokyo Night
//	> Nord
func PromptForPositionalArg(argName, promptTitle string, completionFunc CompletionFunc, cmd *cobra.Command, currentArgs []string) (string, error) {
	defer perf.Track(nil, "flags.PromptForPositionalArg")()

	// Call completion function to get options.
	// Pass current args in case completion is context-dependent (e.g., stack completion depends on component).
	options, _ := completionFunc(cmd, currentArgs, "")
	if len(options) == 0 {
		return "", nil // No options available, let Cobra handle the error
	}

	// PromptForValue will check isInteractive() internally
	return PromptForValue(argName, promptTitle, options)
}
