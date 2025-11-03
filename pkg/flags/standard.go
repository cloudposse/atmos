package flags

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
)

// StandardFlagParser implements FlagParser for regular commands.
// This parser is used for commands that don't pass arguments to external tools
// (e.g., version, describe, list, validate).
//
// Features:
//   - Registers flags with Cobra
//   - Binds flags to Viper for automatic precedence (flag > env > config > default)
//   - Supports NoOptDefVal for identity pattern
//   - Pure function parsing (no side effects)
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
type StandardFlagParser struct {
	registry       *FlagRegistry
	viper          *viper.Viper // Viper instance for precedence handling
	viperPrefix    string
	validValues    map[string][]string // Valid values for flags (flag name -> valid values)
	validationMsgs map[string]string   // Custom validation error messages (flag name -> message)
}

// NewStandardFlagParser creates a new StandardFlagParser with the given options.
//
// Example:
//
//	parser := flagparser.NewStandardFlagParser(
//	    flagparser.WithStackFlag(),
//	    flagparser.WithIdentityFlag(),
//	    flagparser.WithStringFlag("format", "f", "yaml", "Output format"),
//	)
func NewStandardFlagParser(opts ...Option) *StandardFlagParser {
	defer perf.Track(nil, "flagparser.NewStandardFlagParser")()

	config := &parserConfig{
		registry: NewFlagRegistry(),
	}

	// Apply options
	for _, opt := range opts {
		opt(config)
	}

	return &StandardFlagParser{
		registry:    config.registry,
		viperPrefix: config.viperPrefix,
		// TODO: Add validation support when WithValidValues is implemented.
		// validValues:    config.validValues,
		// validationMsgs: config.validationMsgs,
	}
}

// RegisterFlags implements FlagParser.
// Automatically sets DisableFlagParsing=true to ensure our parser handles flag parsing
// instead of Cobra, which allows proper positional arg extraction and Viper precedence.
func (p *StandardFlagParser) RegisterFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flagparser.StandardFlagParser.RegisterFlags")()

	// IMPORTANT: Disable Cobra's flag parsing so our parser can handle it.
	// This is critical for:
	// - Proper positional argument extraction (component names, workflow names, etc.)
	// - Viper precedence handling (CLI → ENV → config → defaults)
	// - Short flag support (-s, -f, -i, etc.)
	cmd.DisableFlagParsing = true

	for _, flag := range p.registry.All() {
		p.registerFlag(cmd, flag)
	}

	// Auto-register completion functions for flags with valid values.
	p.registerCompletions(cmd)
}

// registerFlag registers a single flag with Cobra based on its type.
func (p *StandardFlagParser) registerFlag(cmd *cobra.Command, flag Flag) {
	switch f := flag.(type) {
	case *StringFlag:
		cmd.Flags().StringP(f.Name, f.Shorthand, f.Default, f.Description)

		// Set NoOptDefVal if specified (identity pattern)
		if f.NoOptDefVal != "" {
			cobraFlag := cmd.Flags().Lookup(f.Name)
			if cobraFlag != nil {
				cobraFlag.NoOptDefVal = f.NoOptDefVal
			}
		}

		// Mark as required if needed
		if f.Required {
			_ = cmd.MarkFlagRequired(f.Name)
		}

	case *BoolFlag:
		cmd.Flags().BoolP(f.Name, f.Shorthand, f.Default, f.Description)

	case *IntFlag:
		cmd.Flags().IntP(f.Name, f.Shorthand, f.Default, f.Description)

		if f.Required {
			_ = cmd.MarkFlagRequired(f.Name)
		}

	case *StringSliceFlag:
		cmd.Flags().StringSliceP(f.Name, f.Shorthand, f.Default, f.Description)

		if f.Required {
			_ = cmd.MarkFlagRequired(f.Name)
		}

	default:
		// Unknown flag type - skip
		// In production, this could log a warning
	}
}

// registerCompletions automatically registers shell completion functions
// for flags that have valid values configured.
func (p *StandardFlagParser) registerCompletions(cmd *cobra.Command) {
	if p.validValues == nil || len(p.validValues) == 0 {
		return
	}

	for flagName, validValues := range p.validValues {
		// Only register if the flag actually exists.
		if cmd.Flags().Lookup(flagName) == nil {
			continue
		}

		// Create a closure to capture the validValues for this specific flag.
		values := validValues
		_ = cmd.RegisterFlagCompletionFunc(flagName, func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return values, cobra.ShellCompDirectiveNoFileComp
		})
	}
}

// BindToViper implements FlagParser.
func (p *StandardFlagParser) BindToViper(v *viper.Viper) error {
	defer perf.Track(nil, "flagparser.StandardFlagParser.BindToViper")()

	// Store Viper instance for precedence handling in Parse()
	p.viper = v

	for _, flag := range p.registry.All() {
		if err := p.bindFlag(v, flag); err != nil {
			return err
		}
	}

	return nil
}

// bindFlag binds a single flag to Viper with environment variable support.
func (p *StandardFlagParser) bindFlag(v *viper.Viper, flag Flag) error {
	viperKey := p.getViperKey(flag.GetName())
	return bindFlagToViper(v, viperKey, flag)
}

// BindFlagsToViper is called after RegisterFlags to bind Cobra flags to Viper.
// This must be called separately because we need access to the Cobra command.
//
// Usage:
//
//	parser.RegisterFlags(cmd)
//	parser.BindFlagsToViper(cmd, viper.GetViper())
func (p *StandardFlagParser) BindFlagsToViper(cmd *cobra.Command, v *viper.Viper) error {
	defer perf.Track(nil, "flagparser.StandardFlagParser.BindFlagsToViper")()

	for _, flag := range p.registry.All() {
		viperKey := p.getViperKey(flag.GetName())
		cobraFlag := cmd.Flags().Lookup(flag.GetName())
		if cobraFlag == nil {
			continue
		}

		if err := v.BindPFlag(viperKey, cobraFlag); err != nil {
			return fmt.Errorf("failed to bind flag %s to viper: %w", flag.GetName(), err)
		}
	}

	return nil
}

// Parse implements FlagParser.
func (p *StandardFlagParser) Parse(ctx context.Context, args []string) (*ParsedConfig, error) {
	defer perf.Track(nil, "flagparser.StandardFlagParser.Parse")()

	result := &ParsedConfig{
		Flags:           make(map[string]interface{}),
		PositionalArgs:  args, // For standard commands, all args are positional (e.g., component name)
		PassThroughArgs: []string{},
	}

	// Populate AtmosFlags from Viper with precedence applied
	// Viper contains: CLI flags > ENV vars > config files > defaults
	if p.viper != nil {
		for _, flag := range p.registry.All() {
			flagName := flag.GetName()
			viperKey := p.getViperKey(flagName)
			if p.viper.IsSet(viperKey) {
				// Use type-specific getters to ensure proper type conversion
				// (ENV vars come in as strings and need conversion)
				switch flag.(type) {
				case *BoolFlag:
					result.Flags[flagName] = p.viper.GetBool(viperKey)
				case *IntFlag:
					result.Flags[flagName] = p.viper.GetInt(viperKey)
				case *StringFlag:
					result.Flags[flagName] = p.viper.GetString(viperKey)
				case *StringSliceFlag:
					result.Flags[flagName] = p.viper.GetStringSlice(viperKey)
				default:
					// Fallback for unknown types
					result.Flags[flagName] = p.viper.Get(viperKey)
				}
			}
		}
	}

	// Validate flag values against valid values constraints.
	if err := p.validateFlagValues(result.Flags); err != nil {
		return nil, err
	}

	return result, nil
}

// validateFlagValues validates flag values against configured valid values constraints.
// Returns error if any flag value is not in its valid values list.
func (p *StandardFlagParser) validateFlagValues(flags map[string]interface{}) error {
	defer perf.Track(nil, "flagparser.StandardFlagParser.validateFlagValues")()

	if p.validValues == nil {
		return nil
	}

	for flagName, validValues := range p.validValues {
		value, exists := flags[flagName]
		if !exists {
			continue
		}

		// Convert value to string for comparison.
		strValue, ok := value.(string)
		if !ok {
			continue // Only validate string flags
		}

		// Skip empty values (not set).
		if strValue == "" {
			continue
		}

		// Check if value is in valid values list.
		valid := false
		for _, validValue := range validValues {
			if strValue == validValue {
				valid = true
				break
			}
		}

		if !valid {
			// Check for custom error message.
			if msg, hasMsg := p.validationMsgs[flagName]; hasMsg {
				return fmt.Errorf("%s", msg)
			}
			// Default error message.
			return fmt.Errorf("invalid value %q for flag --%s (valid values: %s)",
				strValue, flagName, strings.Join(validValues, ", "))
		}
	}

	return nil
}

// getViperKey returns the Viper key for a flag name.
// If a prefix is set, it's prepended to the flag name.
func (p *StandardFlagParser) getViperKey(flagName string) string {
	if p.viperPrefix != "" {
		return p.viperPrefix + "." + flagName
	}
	return flagName
}

// GetIdentityFromCmd retrieves the identity value from a command with proper precedence.
// This handles the NoOptDefVal pattern for the identity flag.
//
// Precedence:
//  1. Flag value (if changed)
//  2. Environment variable (ATMOS_IDENTITY, IDENTITY)
//  3. Config file
//  4. Default (empty string)
//
// Special handling:
//   - If flag value is cfg.IdentityFlagSelectValue, triggers interactive selection
//   - If flag not changed, falls back to Viper (env/config)
//
// Usage:
//
//	identity, err := parser.GetIdentityFromCmd(cmd, viper.GetViper())
//	if err != nil {
//	    return err
//	}
//	if identity == cfg.IdentityFlagSelectValue {
//	    // Show interactive selector
//	}
func (p *StandardFlagParser) GetIdentityFromCmd(cmd *cobra.Command, v *viper.Viper) (string, error) {
	defer perf.Track(nil, "flagparser.StandardFlagParser.GetIdentityFromCmd")()

	flagName := cfg.IdentityFlagName

	// Check if flag was explicitly set (highest priority)
	if cmd.Flags().Changed(flagName) {
		flagValue, err := cmd.Flags().GetString(flagName)
		if err != nil {
			return "", fmt.Errorf("failed to get identity flag: %w", err)
		}
		return flagValue, nil
	}

	// Flag not changed - fall back to Viper (env var or config)
	viperKey := p.getViperKey(flagName)
	return v.GetString(viperKey), nil
}
