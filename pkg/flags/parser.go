package flags

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

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
// DEPRECATED: The map-based Flags field is deprecated. Use the strongly-typed
// options methods instead: ToTerraformOptions(), ToHelmfileOptions(), etc.
//
// This type exists for backward compatibility during migration. Eventually, Parse()
// methods will return options directly.
type ParsedConfig struct {
	// Flags contains parsed Atmos-specific flags (--stack, --identity, etc.).
	// Keys are flag names, values are the parsed values.
	//
	// DEPRECATED: Use ToTerraformOptions() for type-safe access instead.
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
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetStringSlice extracts a string slice value from the parsed flags map.
func GetStringSlice(m map[string]interface{}, key string) []string {
	if v, ok := m[key]; ok {
		if slice, ok := v.([]string); ok {
			return slice
		}
	}
	return nil
}

// GetBool extracts a boolean value from the parsed flags map.
func GetBool(m map[string]interface{}, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// GetInt extracts an integer value from the parsed flags map.
func GetInt(m map[string]interface{}, key string) int {
	if v, ok := m[key]; ok {
		if i, ok := v.(int); ok {
			return i
		}
	}
	return 0
}

//nolint:unparam // key parameter kept for consistency with other getter functions
// GetIdentitySelector extracts an IdentitySelector value from the parsed flags map.
func GetIdentitySelector(m map[string]interface{}, key string) IdentitySelector {
	value := GetString(m, key)
	// Check if identity was explicitly provided by checking if the key exists.
	_, provided := m[key]
	return NewIdentitySelector(value, provided)
}

//nolint:unparam // key parameter kept for consistency with other getter functions
// GetPagerSelector extracts a PagerSelector value from the parsed flags map.
func GetPagerSelector(m map[string]interface{}, key string) PagerSelector {
	value := GetString(m, key)
	// Check if pager was explicitly provided by checking if the key exists.
	_, provided := m[key]
	return NewPagerSelector(value, provided)
}
