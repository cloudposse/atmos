package flags

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/perf"
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

	// Reset clears any internal parser state to prevent pollution between test runs.
	// This should be called between tests when reusing a global parser instance.
	// For production code, parsers are typically created once and don't need resetting.
	Reset()
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
//
// Deprecated: The map-based Flags field is deprecated. Use the strongly-typed
// options methods instead: ToTerraformOptions(), ToHelmfileOptions(), etc.
//
// This type exists for backward compatibility during migration. Eventually, Parse()
// methods will return options directly.
type ParsedConfig struct {
	// Flags contains parsed Atmos-specific flags (--stack, --identity, etc.).
	// Keys are flag names, values are the parsed values.
	//
	// Deprecated: Use ToTerraformOptions() for type-safe access instead.
	// This map will be removed in a future version.
	Flags map[string]interface{}

	// PositionalArgs contains positional arguments extracted from the command line.
	// The meaning of these depends on the command:
	//   - For terraform: [component] e.g., ["vpc"]
	//   - For packer/helmfile: [component] e.g., ["vpc"]
	// Callers should interpret these based on their command's semantics.
	PositionalArgs []string

	// SeparatedArgs contains arguments after the -- separator for external tools.
	// These arguments come after -- and are passed to external tools unchanged.
	// Example: atmos terraform plan vpc -s dev -- -var foo=bar
	//   PositionalArgs: ["vpc"]
	//   SeparatedArgs: ["-var", "foo=bar"]
	SeparatedArgs []string
}

// GetIdentity returns the identity value from parsed flags with proper type safety.
// Returns empty string if identity is not set.
func (p *ParsedConfig) GetIdentity() string {
	defer perf.Track(nil, "flags.ParsedConfig.GetIdentity")()

	return GetString(p.Flags, "identity")
}

// GetStack returns the stack value from parsed flags with proper type safety.
// Returns empty string if stack is not set.
func (p *ParsedConfig) GetStack() string {
	defer perf.Track(nil, "flags.ParsedConfig.GetStack")()

	return GetString(p.Flags, "stack")
}

// NOTE: ToTerraformOptions() was removed to avoid circular dependency between
// pkg/flags and pkg/flags/terraform.
//
// To convert ParsedConfig to terraform.Options, use terraform.ParseFlags() or
// construct terraform.Options directly from the ParsedConfig.Flags map.

// Helper functions for safe map access with type conversion.

// GetString extracts a string value from the parsed flags map.
func GetString(m map[string]interface{}, key string) string {
	defer perf.Track(nil, "flags.GetString")()

	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetStringSlice extracts a string slice value from the parsed flags map.
func GetStringSlice(m map[string]interface{}, key string) []string {
	defer perf.Track(nil, "flags.GetStringSlice")()

	if v, ok := m[key]; ok {
		if slice, ok := v.([]string); ok {
			return slice
		}
	}
	return nil
}

// GetBool extracts a boolean value from the parsed flags map.
func GetBool(m map[string]interface{}, key string) bool {
	defer perf.Track(nil, "flags.GetBool")()

	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// GetInt extracts an integer value from the parsed flags map.
func GetInt(m map[string]interface{}, key string) int {
	defer perf.Track(nil, "flags.GetInt")()

	if v, ok := m[key]; ok {
		if i, ok := v.(int); ok {
			return i
		}
	}
	return 0
}

// GetIdentitySelector extracts an IdentitySelector value from the parsed flags map.
//
//nolint:unparam // key parameter kept for consistency with other getter functions
func GetIdentitySelector(m map[string]interface{}, key string) global.IdentitySelector {
	defer perf.Track(nil, "flags.GetIdentitySelector")()

	value := GetString(m, key)
	// Check if identity was explicitly provided by checking if the key exists.
	_, provided := m[key]
	return global.NewIdentitySelector(value, provided)
}

// GetPagerSelector extracts a PagerSelector value from the parsed flags map.
//
//nolint:unparam // key parameter kept for consistency with other getter functions
func GetPagerSelector(m map[string]interface{}, key string) global.PagerSelector {
	defer perf.Track(nil, "flags.GetPagerSelector")()

	value := GetString(m, key)
	// Check if pager was explicitly provided by checking if the key exists.
	_, provided := m[key]
	return global.NewPagerSelector(value, provided)
}

// ResetCommandFlags resets all flags on a cobra.Command to their default values.
// This is a shared helper used by all FlagParser implementations to prevent
// flag pollution between test runs.
//
// It resets:
//   - Local flags (cmd.Flags())
//   - Persistent flags (cmd.PersistentFlags())
//
// For each flag, it:
//  1. Sets the value back to DefValue
//  2. Clears the Changed state
func ResetCommandFlags(cmd *cobra.Command) {
	if cmd == nil {
		return
	}

	resetFlagSet := func(flagSet *pflag.FlagSet) {
		flagSet.VisitAll(func(flag *pflag.Flag) {
			// Reset to default value.
			_ = flag.Value.Set(flag.DefValue)
			// Clear Changed state.
			flag.Changed = false
		})
	}

	// Reset both local and persistent flags.
	resetFlagSet(cmd.Flags())
	resetFlagSet(cmd.PersistentFlags())
}
