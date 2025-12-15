package flags

import (
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/term"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosterm "github.com/cloudposse/atmos/internal/tui/templates/term"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Selector height constants.
const (
	// MaxSelectorHeight is the maximum number of rows for the interactive selector.
	maxSelectorHeight = 20
	// MinSelectorHeight is the minimum number of rows to show in the selector.
	minSelectorHeight = 3
	// SelectorPadding accounts for title and borders in the selector.
	selectorPadding = 2
	// TerminalReservedRows reserves space for prompt line and some buffer.
	terminalReservedRows = 5
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
	return atmosterm.IsTTYSupportForStdin() && !telemetry.IsCI()
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

	// Calculate dynamic height based on options and terminal size.
	height := calculateSelectorHeight(len(options))

	// Create Huh selector with Atmos theme.
	// Note: Huh v0.8.0 has case-sensitive filtering (by design).
	// Users can filter by typing "/" followed by search text, but it only matches exact case.
	// Example: typing "dark" matches "neobones_dark" but not "Builtin Dark".
	// TODO: Consider filing upstream feature request for case-insensitive filtering option.
	selector := huh.NewSelect[string]().
		Value(&choice).
		Options(huh.NewOptions(options...)...).
		Title(title).
		Height(height).
		WithTheme(uiutils.NewAtmosHuhTheme())

	// Run selector.
	if err := selector.Run(); err != nil {
		return "", fmt.Errorf("prompt failed: %w", err)
	}

	// Show what was selected for terminal history visibility.
	_ = ui.Infof("Selected %s: %s", name, choice)

	return choice, nil
}

// calculateSelectorHeight determines the optimal height for the selector.
// It considers: number of options, terminal height, and min/max bounds.
func calculateSelectorHeight(optionCount int) int {
	// Start with options + padding for title/borders.
	height := optionCount + selectorPadding

	// Try to get terminal height for smarter sizing.
	if _, termHeight, err := term.GetSize(int(os.Stdout.Fd())); err == nil && termHeight > 0 {
		// Calculate available space (terminal height minus reserved rows).
		available := termHeight - terminalReservedRows
		if available < minSelectorHeight {
			available = minSelectorHeight
		}
		// Use the smaller of calculated height and available space.
		if height > available {
			height = available
		}
	}

	// Apply min/max bounds.
	if height < minSelectorHeight {
		height = minSelectorHeight
	}
	if height > maxSelectorHeight {
		height = maxSelectorHeight
	}

	return height
}

// PromptForMissingRequired prompts for a required flag that is missing.
// This is Use Case 1: Missing Required Flags.
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
		return "", nil // Gracefully return empty - Cobra will handle the error.
	}

	// Call completion function to get options.
	options, _ := completionFunc(cmd, args, "")
	if len(options) == 0 {
		return "", nil // No options available, let Cobra handle the error.
	}

	return PromptForValue(flagName, promptTitle, options)
}

// OptionalValuePromptContext holds the context for prompting when a flag is used without a value.
type OptionalValuePromptContext struct {
	FlagName       string
	FlagValue      string
	PromptTitle    string
	CompletionFunc CompletionFunc
	Cmd            *cobra.Command
	Args           []string
}

// PromptForOptionalValue prompts for a flag that was used without a value.
// This is Use Case 2: Optional Value Flags (like the --identity pattern).
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
func PromptForOptionalValue(ctx *OptionalValuePromptContext) (string, error) {
	defer perf.Track(nil, "flags.PromptForOptionalValue")()

	if ctx == nil {
		return "", fmt.Errorf("%w: optional value prompt context", errUtils.ErrNilInput)
	}

	// Check if flag value matches the sentinel (indicating user wants interactive selection).
	if ctx.FlagValue != cfg.IdentityFlagSelectValue {
		return ctx.FlagValue, nil // Real value provided, no prompt needed.
	}

	if !isInteractive() {
		return "", nil // Gracefully return empty - command can use default.
	}

	// Call completion function to get options.
	options, _ := ctx.CompletionFunc(ctx.Cmd, ctx.Args, "")
	if len(options) == 0 {
		return "", nil // No options available, use default.
	}

	return PromptForValue(ctx.FlagName, ctx.PromptTitle, options)
}

// PromptForPositionalArg prompts for a required positional argument that is missing.
// This is Use Case 3: Missing Required Positional Arguments.
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

	if !isInteractive() {
		return "", nil // Gracefully return empty - Cobra will handle the error.
	}

	// Call completion function to get options.
	// Pass current args in case completion is context-dependent (e.g., stack completion depends on component).
	options, _ := completionFunc(cmd, currentArgs, "")
	if len(options) == 0 {
		return "", nil // No options available, let Cobra handle the error.
	}

	return PromptForValue(argName, promptTitle, options)
}
