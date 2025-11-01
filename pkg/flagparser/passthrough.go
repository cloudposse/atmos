package flagparser

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// FlagPrefix is the prefix for flag arguments.
	flagPrefix = "-"
)

// PassThroughFlagParser implements FlagParser for pass-through commands.
// This parser is used for commands that pass arguments to external tools
// (e.g., terraform, helmfile, packer).
//
// Two-phase parsing:
//
//	Phase 1: Extract Atmos-specific flags (--stack, --identity, --dry-run, etc.)
//	Phase 2: Pass remaining arguments to external tool unchanged
//
// Two modes:
//
//	Explicit mode: With -- separator (recommended)
//	  Example: atmos terraform plan vpc -s dev -- -var foo=bar
//	Implicit mode: Without -- separator (backward compatibility)
//	  Example: atmos terraform plan vpc -s dev -var foo=bar
//
// Usage:
//
//	parser := flagparser.NewPassThroughFlagParser(
//	    flagparser.WithTerraformFlags(),
//	)
//
//	// In command setup:
//	parser.RegisterFlags(cmd)
//	parser.BindToViper(viper.GetViper())
//
//	// In command execution:
//	cfg, err := parser.Parse(ctx, args)
//	// cfg.AtmosFlags contains Atmos flags
//	// cfg.PassThroughArgs contains args to pass to terraform
type PassThroughFlagParser struct {
	registry          *FlagRegistry
	viperPrefix       string
	atmosFlagNames    []string          // Known Atmos flag names for extraction
	shorthandToFull   map[string]string // Maps shorthand (e.g., "s") to full name (e.g., "stack")
	optionalBoolFlags []string          // Flags that support --flag or --flag=value
}

// NewPassThroughFlagParser creates a new PassThroughFlagParser with the given options.
//
// Example:
//
//	parser := flagparser.NewPassThroughFlagParser(
//	    flagparser.WithTerraformFlags(),
//	)
func NewPassThroughFlagParser(opts ...Option) *PassThroughFlagParser {
	defer perf.Track(nil, "flagparser.NewPassThroughFlagParser")()

	config := &parserConfig{
		registry: NewFlagRegistry(),
	}

	// Apply options
	for _, opt := range opts {
		opt(config)
	}

	// Build list of known Atmos flag names for extraction
	atmosFlagNames := make([]string, 0, config.registry.Count())
	shorthandToFull := make(map[string]string)
	optionalBoolFlags := make([]string, 0)

	for _, flag := range config.registry.All() {
		atmosFlagNames = append(atmosFlagNames, flag.GetName())
		if flag.GetShorthand() != "" {
			atmosFlagNames = append(atmosFlagNames, flag.GetShorthand())
			// Map shorthand to full name (e.g., "s" -> "stack")
			shorthandToFull[flag.GetShorthand()] = flag.GetName()
		}

		// Track optional bool flags (like --upload-status)
		if boolFlag, ok := flag.(*BoolFlag); ok {
			optionalBoolFlags = append(optionalBoolFlags, boolFlag.Name)
		}
	}

	return &PassThroughFlagParser{
		registry:          config.registry,
		viperPrefix:       config.viperPrefix,
		atmosFlagNames:    atmosFlagNames,
		shorthandToFull:   shorthandToFull,
		optionalBoolFlags: optionalBoolFlags,
	}
}

// RegisterFlags implements FlagParser.
func (p *PassThroughFlagParser) RegisterFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flagparser.PassThroughFlagParser.RegisterFlags")()

	for _, flag := range p.registry.All() {
		p.registerFlag(cmd, flag)
	}
}

// registerFlag registers a single flag with Cobra based on its type.
func (p *PassThroughFlagParser) registerFlag(cmd *cobra.Command, flag Flag) {
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
	}
}

// BindToViper implements FlagParser.
func (p *PassThroughFlagParser) BindToViper(v *viper.Viper) error {
	defer perf.Track(nil, "flagparser.PassThroughFlagParser.BindToViper")()

	for _, flag := range p.registry.All() {
		if err := p.bindFlag(v, flag); err != nil {
			return err
		}
	}

	return nil
}

// bindFlag binds a single flag to Viper with environment variable support.
func (p *PassThroughFlagParser) bindFlag(v *viper.Viper, flag Flag) error {
	viperKey := p.getViperKey(flag.GetName())
	return bindFlagToViper(v, viperKey, flag)
}

// BindFlagsToViper binds Cobra flags to Viper after command initialization.
// This must be called after RegisterFlags.
func (p *PassThroughFlagParser) BindFlagsToViper(cmd *cobra.Command, v *viper.Viper) error {
	defer perf.Track(nil, "flagparser.PassThroughFlagParser.BindFlagsToViper")()

	for _, flag := range p.registry.All() {
		// Skip NoOptDefVal flags - they're handled manually
		if flag.GetNoOptDefVal() != "" {
			continue
		}

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
// This is the core two-phase parsing logic:
//
//	Phase 1: Extract Atmos flags from mixed args
//	Phase 2: Return pass-through args for external tool
//
// revive:disable-next-line:function-length Core parsing logic with many edge cases.
func (p *PassThroughFlagParser) Parse(ctx context.Context, args []string) (*ParsedConfig, error) {
	defer perf.Track(nil, "flagparser.PassThroughFlagParser.Parse")()

	result := &ParsedConfig{
		AtmosFlags: make(map[string]interface{}),
	}

	// Check for explicit double-dash separator
	beforeDash, afterDash := p.SplitAtDoubleDash(args)

	var atmosArgs, toolArgs []string

	if afterDash != nil {
		// Explicit mode: -- separator present
		// Everything after -- goes to tool unchanged
		atmosArgs = beforeDash
		toolArgs = afterDash
	} else {
		// Implicit mode: no -- separator
		// Extract Atmos flags, everything else goes to tool
		atmosArgs = args
		toolArgs = nil
	}

	// Extract Atmos flags from atmosArgs
	// This handles both explicit mode (with --) and implicit mode (without --)
	atmosFlagsMap, remaining, err := p.ExtractAtmosFlags(atmosArgs)
	if err != nil {
		return nil, err
	}

	result.AtmosFlags = atmosFlagsMap

	// In explicit mode, prepend remaining args to toolArgs
	// In implicit mode, remaining args ARE the tool args
	if afterDash != nil {
		toolArgs = append(remaining, toolArgs...)
	} else {
		toolArgs = remaining
	}

	// Extract positional arguments (subcommand, component)
	// Expected pattern: terraform plan vpc
	//                   ^^^^^^^^^^^^^^ ^^^
	//                   subcommand     component
	positional, remainingTool, err := p.ExtractPositionalArgs(toolArgs, 2)
	if err != nil {
		// Not an error - some commands don't have positional args
		positional = nil
		remainingTool = toolArgs
	}

	if len(positional) > 0 {
		result.SubCommand = positional[0]
	}
	if len(positional) > 1 {
		result.ComponentName = positional[1]
	}

	result.PassThroughArgs = remainingTool

	return result, nil
}

// SplitAtDoubleDash implements PassThroughHandler.
func (p *PassThroughFlagParser) SplitAtDoubleDash(args []string) (beforeDash, afterDash []string) {
	defer perf.Track(nil, "flagparser.PassThroughFlagParser.SplitAtDoubleDash")()

	separatorIndex := lo.IndexOf(args, "--")
	if separatorIndex < 0 {
		// No separator found
		return args, nil
	}

	// Split at separator (exclude the -- itself)
	beforeDash = lo.Slice(args, 0, separatorIndex)
	afterDash = lo.Slice(args, separatorIndex+1, len(args))

	return beforeDash, afterDash
}

// ExtractAtmosFlags implements PassThroughHandler.
// This extracts known Atmos flags from a mixed argument list.
func (p *PassThroughFlagParser) ExtractAtmosFlags(args []string) (atmosFlags map[string]interface{}, remainingArgs []string, err error) {
	defer perf.Track(nil, "flagparser.PassThroughFlagParser.ExtractAtmosFlags")()

	atmosFlags = make(map[string]interface{})
	remainingArgs = make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]

		// Check if this is an Atmos flag
		if !strings.HasPrefix(arg, flagPrefix) {
			// Not a flag - it's a positional arg or tool flag value
			remainingArgs = append(remainingArgs, arg)
			continue
		}

		// Try to parse as Atmos flag
		flagName, flagValue, consumed, isAtmosFlag := p.parseFlag(args, i)

		if !isAtmosFlag {
			// Not an Atmos flag - pass through
			remainingArgs = append(remainingArgs, arg)
			continue
		}

		// Normalize shorthand to full name (e.g., "s" -> "stack")
		if fullName, isShorthand := p.shorthandToFull[flagName]; isShorthand {
			flagName = fullName
		}

		// Store the Atmos flag with full name
		atmosFlags[flagName] = flagValue

		// Skip consumed args (for --flag value form)
		i += consumed
	}

	return atmosFlags, remainingArgs, nil
}

// parseFlag attempts to parse an argument as an Atmos flag.
// Returns:
//   - flagName: The flag name (without dashes)
//   - flagValue: The flag value
//   - consumed: Number of additional args consumed (0 for --flag=value, 1 for --flag value)
//   - isAtmosFlag: Whether this is an Atmos flag
//
// revive:disable-next-line:function-length,cyclomatic,function-result-limit Complex parsing logic with multiple flag forms.
func (p *PassThroughFlagParser) parseFlag(args []string, index int) (flagName string, flagValue interface{}, consumed int, isAtmosFlag bool) {
	arg := args[index]

	// Handle --flag=value form
	if strings.Contains(arg, "=") {
		parts := strings.SplitN(arg, "=", 2)
		prefix := parts[0]
		value := parts[1]

		// Strip dashes
		name := strings.TrimPrefix(prefix, "--")
		name = strings.TrimPrefix(name, "-")

		if !p.isAtmosFlag(name) {
			return "", nil, 0, false
		}

		return name, value, 0, true
	}

	// Handle --flag value or --flag (NoOptDefVal) form
	name := strings.TrimPrefix(arg, "--")
	name = strings.TrimPrefix(name, "-")

	if !p.isAtmosFlag(name) {
		return "", nil, 0, false
	}

	// Normalize shorthand to full name for registry lookup
	// (registry stores flags by full name only)
	lookupName := name
	if fullName, isShorthand := p.shorthandToFull[name]; isShorthand {
		lookupName = fullName
	}

	// Get the flag definition to check type
	flag := p.registry.Get(lookupName)
	if flag == nil {
		// Shouldn't happen, but handle gracefully
		return "", nil, 0, false
	}

	// Check if this is a boolean flag
	if _, isBool := flag.(*BoolFlag); isBool {
		// Boolean flags don't consume next arg
		return name, true, 0, true
	}

	// Check if this flag has NoOptDefVal (identity pattern)
	if noOptDefVal := flag.GetNoOptDefVal(); noOptDefVal != "" {
		// Check if next arg exists and is not a flag
		if index+1 < len(args) && !strings.HasPrefix(args[index+1], flagPrefix) {
			// Has value: --identity value
			return name, args[index+1], 1, true
		}
		// No value: --identity (use NoOptDefVal)
		return name, noOptDefVal, 0, true
	}

	// String or int flag - consume next arg as value
	if index+1 < len(args) {
		return name, args[index+1], 1, true
	}

	// Flag provided but no value - error case
	// For now, treat as flag without value
	return name, "", 0, true
}

// isAtmosFlag checks if a flag name is a known Atmos flag.
func (p *PassThroughFlagParser) isAtmosFlag(name string) bool {
	return lo.Contains(p.atmosFlagNames, name)
}

// ExtractPositionalArgs implements PassThroughHandler.
func (p *PassThroughFlagParser) ExtractPositionalArgs(args []string, expectedCount int) (positional, remaining []string, err error) {
	defer perf.Track(nil, "flagparser.PassThroughFlagParser.ExtractPositionalArgs")()

	positional = make([]string, 0, expectedCount)
	remaining = make([]string, 0)

	hitFlag := false
	for _, arg := range args {
		// Once we encounter a flag, everything goes to remaining
		if strings.HasPrefix(arg, flagPrefix) {
			hitFlag = true
		}

		// Collect positional args only before hitting any flags
		if !hitFlag && !strings.HasPrefix(arg, flagPrefix) && len(positional) < expectedCount {
			positional = append(positional, arg)
		} else {
			remaining = append(remaining, arg)
		}
	}

	return positional, remaining, nil
}

// getViperKey returns the Viper key for a flag name.
func (p *PassThroughFlagParser) getViperKey(flagName string) string {
	if p.viperPrefix != "" {
		return p.viperPrefix + "." + flagName
	}
	return flagName
}

// GetIdentityFromCmd retrieves the identity value with proper precedence.
// Same as StandardFlagParser.GetIdentityFromCmd.
func (p *PassThroughFlagParser) GetIdentityFromCmd(cmd *cobra.Command, v *viper.Viper) (string, error) {
	defer perf.Track(nil, "flagparser.PassThroughFlagParser.GetIdentityFromCmd")()

	flagName := cfg.IdentityFlagName

	// Check if flag was explicitly set
	if cmd.Flags().Changed(flagName) {
		flagValue, err := cmd.Flags().GetString(flagName)
		if err != nil {
			return "", fmt.Errorf("failed to get identity flag: %w", err)
		}
		return flagValue, nil
	}

	// Fall back to Viper
	viperKey := p.getViperKey(flagName)
	return v.GetString(viperKey), nil
}
