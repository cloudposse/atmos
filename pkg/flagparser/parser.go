package flagparser

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// FlagParser handles flag registration, parsing, and Viper binding for commands.
//
// This interface provides a unified way to handle command-line flags across all
// Atmos commands, ensuring consistent precedence order (flags > env vars > config > defaults),
// proper Viper integration, and support for advanced patterns like NoOptDefVal.
//
// Usage:
//
//	parser := flagparser.NewStandardFlagParser(
//	    flagparser.WithStringFlag("stack", "s", "", "Stack name"),
//	    flagparser.WithBoolFlag("dry-run", "", false, "Dry run mode"),
//	)
//
//	// In command setup:
//	parser.RegisterFlags(cmd)
//	parser.BindToViper(viper.GetViper())
//
//	// In command execution:
//	cfg, err := parser.Parse(ctx, args)
type FlagParser interface {
	// RegisterFlags adds flags to the Cobra command.
	// This should be called during command initialization before the command is added to root.
	RegisterFlags(cmd *cobra.Command)

	// BindToViper binds registered flags to Viper for automatic precedence handling.
	// Must be called after RegisterFlags and before command execution.
	// Handles both flag binding (BindPFlag) and environment variable binding (BindEnv).
	//
	// Special handling for NoOptDefVal flags:
	//   - Only bind env vars, NOT the flag itself
	//   - This prevents Viper from interfering with NoOptDefVal detection
	BindToViper(v *viper.Viper) error

	// Parse processes command-line arguments and returns parsed configuration.
	// For standard commands, this extracts flag values from Viper.
	// For pass-through commands, this separates Atmos flags from tool flags.
	//
	// Returns ParsedConfig containing:
	//   - Atmos flags and their values
	//   - Pass-through arguments (for terraform/helmfile/etc)
	//   - Component and stack information
	Parse(ctx context.Context, args []string) (*ParsedConfig, error)
}

// PassThroughHandler handles the separation of Atmos-specific flags from tool flags.
// This is used by commands that pass arguments to external tools (terraform, helmfile, packer).
//
// Two parsing modes:
//   - Explicit mode: With -- separator (recommended)
//   - Implicit mode: Without -- separator (backward compatibility)
type PassThroughHandler interface {
	// SplitAtDoubleDash separates arguments at the -- marker.
	// Returns (beforeDash, afterDash).
	// If no -- found, afterDash is nil.
	SplitAtDoubleDash(args []string) (beforeDash, afterDash []string)

	// ExtractAtmosFlags pulls known Atmos flags from a mixed argument list.
	// Returns:
	//   - atmosFlags: Map of flag name -> value for Atmos-specific flags
	//   - remainingArgs: Arguments that weren't Atmos flags (tool flags + positional args)
	//   - error: If flag parsing fails
	//
	// This is used in implicit mode (no -- separator) to extract Atmos flags
	// while preserving tool flags exactly as provided.
	ExtractAtmosFlags(args []string) (atmosFlags map[string]interface{}, remainingArgs []string, err error)

	// ExtractPositionalArgs identifies positional arguments from an argument list.
	// expectedCount is the number of positional args expected (e.g., 2 for "terraform plan vpc").
	// Returns:
	//   - positional: The positional arguments found
	//   - remaining: Arguments after positional args (flags)
	//   - error: If not enough positional args found
	ExtractPositionalArgs(args []string, expectedCount int) (positional, remaining []string, err error)
}

// ParsedConfig contains the results of parsing command-line arguments.
// Different fields are populated depending on command type (standard vs pass-through).
type ParsedConfig struct {
	// AtmosFlags contains parsed Atmos-specific flags (--stack, --identity, etc.).
	// Keys are flag names, values are the parsed values.
	AtmosFlags map[string]interface{}

	// PassThroughArgs contains arguments to pass to external tools.
	// Only populated for pass-through commands (terraform, helmfile, packer).
	// These arguments are NOT parsed by Atmos - they're passed directly to the tool.
	PassThroughArgs []string

	// PositionalArgs contains positional arguments extracted from the command line.
	// The meaning of these depends on the command:
	//   - For terraform: [subcommand, component] e.g., ["plan", "vpc"]
	//   - For packer/helmfile: [component] e.g., ["vpc"]
	//   - For auth exec: [] (no positional args, everything is pass-through)
	// Callers should interpret these based on their command's semantics.
	PositionalArgs []string
}
