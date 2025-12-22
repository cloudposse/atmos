package flags

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
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
//	parser := flags.NewStandardFlagParser(
//	    flags.WithStringFlag("stack", "s", "", "Stack name"),
//	    flags.WithBoolFlag("dry-run", "", false, "Dry run mode"),
//	)
//
//	// In command setup:
//	parser.RegisterFlags(cmd)
//	parser.BindToViper(viper.GetViper())
type StandardFlagParser struct {
	registry             *FlagRegistry
	cmd                  *cobra.Command // Command for manual flag parsing
	viper                *viper.Viper   // Viper instance for precedence handling
	viperPrefix          string
	validValues          map[string][]string          // Valid values for flags (flag name -> valid values)
	validationMsgs       map[string]string            // Custom validation error messages (flag name -> message)
	parsedFlags          *pflag.FlagSet               // Combined FlagSet used in last Parse() call (for Changed checks)
	positionalArgs       *positionalArgsConfig        // Positional argument configuration
	flagPrompts          map[string]*flagPromptConfig // Prompt configs for missing required flags (Use Case 1)
	optionalValuePrompts map[string]*flagPromptConfig // Prompt configs for optional value flags (Use Case 2)
	positionalPrompts    map[string]*flagPromptConfig // Prompt configs for missing positional args (Use Case 3)
}

// NewStandardFlagParser creates a new StandardFlagParser with the given options.
//
// Example:
//
//	parser := flags.NewStandardFlagParser(
//	    flags.WithStackFlag(),
//	    flags.WithIdentityFlag(),
//	    flags.WithStringFlag("format", "f", "yaml", "Output format"),
//	)
func NewStandardFlagParser(opts ...Option) *StandardFlagParser {
	defer perf.Track(nil, "flags.NewStandardFlagParser")()

	config := &parserConfig{
		registry:             NewFlagRegistry(),
		flagPrompts:          make(map[string]*flagPromptConfig),
		optionalValuePrompts: make(map[string]*flagPromptConfig),
		positionalPrompts:    make(map[string]*flagPromptConfig),
	}

	// Apply options
	for _, opt := range opts {
		opt(config)
	}

	return &StandardFlagParser{
		registry:             config.registry,
		viperPrefix:          config.viperPrefix,
		validValues:          make(map[string][]string),
		validationMsgs:       make(map[string]string),
		flagPrompts:          config.flagPrompts,
		optionalValuePrompts: config.optionalValuePrompts,
		positionalPrompts:    config.positionalPrompts,
	}
}

// Registry returns the underlying flag registry.
// This allows access to the registry for operations like SetCompletionFunc()
// that need to modify flags after parser creation. Any mutations to the
// returned registry must be made before calling RegisterFlags or
// RegisterPersistentFlags; changes made afterward will not affect
// already-registered flags.
func (p *StandardFlagParser) Registry() *FlagRegistry {
	defer perf.Track(nil, "flags.StandardFlagParser.Registry")()

	return p.registry
}

// SetPositionalArgs configures positional argument extraction and validation.
//
// Parameters:
//   - specs: Positional argument specifications with TargetField mapping
//   - validator: Cobra Args validator function
//   - usage: Usage string for Cobra Use field (e.g., "[component]")
//
// This method is called by StandardOptionsBuilder.Build() when WithPositionalArgs() was used.
func (p *StandardFlagParser) SetPositionalArgs(
	specs []*PositionalArgSpec,
	validator cobra.PositionalArgs,
	usage string,
) {
	defer perf.Track(nil, "flags.StandardFlagParser.SetPositionalArgs")()

	p.positionalArgs = &positionalArgsConfig{
		specs:     specs,
		validator: validator,
		usage:     usage,
	}
}

// ParsedFlags returns the combined FlagSet used in the last Parse() call.
// This is useful for checking if flags were Changed when DisableFlagParsing is enabled.
func (p *StandardFlagParser) ParsedFlags() *pflag.FlagSet {
	defer perf.Track(nil, "flags.StandardFlagParser.ParsedFlags")()

	return p.parsedFlags
}

// RegisterFlags registers flags with Cobra for normal flag validation.
// Does NOT set DisableFlagParsing, allowing Cobra to validate flags and reject unknown ones.
//
// For commands that need to pass unknown flags to external tools (terraform, helmfile, packer),
// those commands should set DisableFlagParsing=true manually in their command definition.
// This is a temporary measure until the compatibility flags system is fully integrated.
//
// If positional args with prompts are configured, this sets a prompt-aware Args validator
// that allows missing required args when interactive mode is available.
func (p *StandardFlagParser) RegisterFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flags.StandardFlagParser.RegisterFlags")()

	// Store command for flag binding and Parse() method
	p.cmd = cmd

	// DO NOT set DisableFlagParsing here.
	// Let Cobra handle flag parsing and validation normally for most commands.
	// Commands that need pass-through (terraform, helmfile, packer) set it manually.
	//
	// Why not set it here:
	// - Compatibility flags (the proper solution) preprocess args BEFORE Cobra sees them
	// - Moved pass-through flags are in separatedArgs, not given to Cobra
	// - Cobra only sees Atmos flags and can validate them normally
	// - Unknown flags are properly rejected by Cobra
	//
	// Legacy behavior (still used by terraform/helmfile/packer):
	// - Commands manually set DisableFlagParsing=true in their definition
	// - This bypasses Cobra validation entirely (less ideal)
	// - Will be replaced by compatibility flags in future PR

	for _, flag := range p.registry.All() {
		p.registerFlag(cmd, flag)
	}

	// Auto-register completion functions for flags with valid values.
	p.registerCompletions(cmd)

	// If positional args with prompts are configured, set prompt-aware validator
	p.registerPositionalArgsValidator(cmd)
}

// registerPositionalArgsValidator sets a prompt-aware Args validator on the command
// if positional args are configured and prompts exist.
//
// This allows missing required positional args when interactive prompts will handle them,
// solving the timing issue where Cobra's Args validation happens before Parse() can prompt.
func (p *StandardFlagParser) registerPositionalArgsValidator(cmd *cobra.Command) {
	defer perf.Track(nil, "flags.StandardFlagParser.registerPositionalArgsValidator")()

	// Only set validator if positional args are configured
	if p.positionalArgs == nil || len(p.positionalArgs.specs) == 0 {
		return
	}

	// Check if any prompts are configured for positional args.
	hasPrompts := false
	for _, spec := range p.positionalArgs.specs {
		if _, exists := p.positionalPrompts[spec.Name]; exists {
			hasPrompts = true
			break
		}
	}

	// Only set prompt-aware validator when prompts are configured.
	// This avoids overriding any pre-existing cmd.Args validator when not needed.
	if hasPrompts {
		builder := NewPositionalArgsBuilder()
		for _, spec := range p.positionalArgs.specs {
			builder.AddArg(spec)
		}
		validator := builder.GeneratePromptAwareValidator(true)
		cmd.Args = validator
	}
}

// GetActualArgs extracts the actual arguments when DisableFlagParsing=true.
// When DisableFlagParsing=true, cmd.Flags().Args() returns empty because Cobra
// doesn't parse flags. This function falls back to os.Args to get the raw arguments.
//
// This logic is extracted here to be testable and reusable, rather than duplicated
// in UsageFunc handlers.
//
// Returns the arguments slice that should be used for Args validation.
func GetActualArgs(cmd *cobra.Command, osArgs []string) []string {
	defer perf.Track(nil, "flags.GetActualArgs")()

	arguments := cmd.Flags().Args()
	if len(arguments) == 0 && cmd.DisableFlagParsing {
		// Extract args from os.Args based on command path depth.
		// For example, "atmos describe component comp1" has path depth 3,
		// so we take osArgs[3:] to get ["comp1"].
		commandDepth := len(strings.Split(cmd.CommandPath(), " "))
		if commandDepth < len(osArgs) {
			arguments = osArgs[commandDepth:]
		}
	}
	return arguments
}

// ValidateArgsOrNil checks if the command's Args validator accepts the given arguments.
// Returns nil if validation passes, or the validation error if it fails.
//
// This is used to distinguish between:
//   - Valid positional arguments (return nil, show usage without "Unknown command" error)
//   - Invalid arguments/unknown subcommands (return error, show "Unknown command" error)
//
// This logic is extracted here to be testable and avoid duplication in UsageFunc handlers.
func ValidateArgsOrNil(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "flags.ValidateArgsOrNil")()

	if cmd.Args == nil {
		return nil // No validator means all args are valid
	}
	return cmd.Args(cmd, args)
}

// registerFlagToSet is a helper that registers a single flag to a pflag.FlagSet.
// This eliminates duplication between registerFlag and registerPersistentFlag.
// The markRequired function allows callers to specify how to mark required flags
// (MarkFlagRequired for regular flags, MarkPersistentFlagRequired for persistent flags).
func (p *StandardFlagParser) registerFlagToSet(flagSet *pflag.FlagSet, flag Flag, markRequired func(string) error) {
	switch f := flag.(type) {
	case *StringFlag:
		p.registerStringFlag(flagSet, f, markRequired)
	case *BoolFlag:
		flagSet.BoolP(f.Name, f.Shorthand, f.Default, f.Description)
	case *IntFlag:
		p.registerIntFlag(flagSet, f, markRequired)
	case *StringSliceFlag:
		p.registerStringSliceFlag(flagSet, f, markRequired)
	default:
		// Unknown flag type - skip.
		// In production, this could log a warning.
	}
}

// registerStringFlag registers a string flag with optional NoOptDefVal and ValidValues.
func (p *StandardFlagParser) registerStringFlag(flagSet *pflag.FlagSet, f *StringFlag, markRequired func(string) error) {
	defer perf.Track(nil, "flags.StandardFlagParser.registerStringFlag")()

	flagSet.StringP(f.Name, f.Shorthand, f.Default, f.Description)

	// Set NoOptDefVal if specified (identity pattern).
	if f.NoOptDefVal != "" {
		cobraFlag := flagSet.Lookup(f.Name)
		if cobraFlag != nil {
			cobraFlag.NoOptDefVal = f.NoOptDefVal
		}
	}

	// Populate validValues map for runtime validation.
	if len(f.ValidValues) > 0 {
		p.validValues[f.Name] = f.ValidValues
	}

	// Mark as required if needed.
	if f.Required {
		_ = markRequired(f.Name)
	}
}

// registerIntFlag registers an integer flag with optional required marking.
func (p *StandardFlagParser) registerIntFlag(flagSet *pflag.FlagSet, f *IntFlag, markRequired func(string) error) {
	defer perf.Track(nil, "flags.StandardFlagParser.registerIntFlag")()

	flagSet.IntP(f.Name, f.Shorthand, f.Default, f.Description)

	if f.Required {
		_ = markRequired(f.Name)
	}
}

// registerStringSliceFlag registers a string slice flag with optional required marking.
func (p *StandardFlagParser) registerStringSliceFlag(flagSet *pflag.FlagSet, f *StringSliceFlag, markRequired func(string) error) {
	defer perf.Track(nil, "flags.StandardFlagParser.registerStringSliceFlag")()

	flagSet.StringSliceP(f.Name, f.Shorthand, f.Default, f.Description)

	if f.Required {
		_ = markRequired(f.Name)
	}
}

// registerFlag registers a single flag with Cobra based on its type.
func (p *StandardFlagParser) registerFlag(cmd *cobra.Command, flag Flag) {
	p.registerFlagToSet(cmd.Flags(), flag, cmd.MarkFlagRequired)
}

// RegisterPersistentFlags registers flags as persistent flags (available to subcommands).
// This is used for global flags that should be inherited by all subcommands.
// NOTE: Unlike RegisterFlags(), this does NOT set DisableFlagParsing=true because
// persistent flags on the root command should work with Cobra's normal flag parsing.
// Disabling flag parsing on the root would break all subcommands' positional arguments.
func (p *StandardFlagParser) RegisterPersistentFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flags.StandardFlagParser.RegisterPersistentFlags")()

	// Store command for manual flag parsing in Parse().
	p.cmd = cmd

	// DO NOT set cmd.DisableFlagParsing = true here!
	// Persistent flags on root command must work with Cobra's normal parsing.
	// Otherwise, all subcommands' positional args will be treated as unknown subcommands.

	for _, flag := range p.registry.All() {
		p.registerPersistentFlag(cmd, flag)
	}

	// Register shell completions for flags with valid values.
	p.registerPersistentCompletions(cmd)
}

// registerPersistentFlag registers a single flag as a persistent flag with Cobra.
func (p *StandardFlagParser) registerPersistentFlag(cmd *cobra.Command, flag Flag) {
	p.registerFlagToSet(cmd.PersistentFlags(), flag, cmd.MarkPersistentFlagRequired)
}

// registerCompletions automatically registers shell completion functions
// for flags that have valid values configured.
func (p *StandardFlagParser) registerCompletions(cmd *cobra.Command) {
	if len(p.validValues) == 0 {
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

// registerPersistentCompletions automatically registers shell completion functions
// for persistent flags that have valid values OR custom completion functions configured.
func (p *StandardFlagParser) registerPersistentCompletions(cmd *cobra.Command) {
	// Register static completions (valid values).
	for flagName, validValues := range p.validValues {
		// Only register if the flag actually exists.
		if cmd.PersistentFlags().Lookup(flagName) == nil {
			continue
		}

		// Create a closure to capture the validValues for this specific flag.
		values := validValues
		_ = cmd.RegisterFlagCompletionFunc(flagName, func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return values, cobra.ShellCompDirectiveNoFileComp
		})
	}

	// Register custom completion functions.
	// For persistent flags, we need to register completion on both the parent command
	// and recursively on all child commands because Cobra doesn't automatically
	// propagate completion functions from parent to children.
	for _, flag := range p.registry.All() {
		flagName := flag.GetName()

		// Only register if the flag actually exists.
		if cmd.PersistentFlags().Lookup(flagName) == nil {
			continue
		}

		// Only register if the flag has a custom completion function.
		if completionFunc := flag.GetCompletionFunc(); completionFunc != nil {
			_ = cmd.RegisterFlagCompletionFunc(flagName, completionFunc)

			// Recursively register on all descendant commands.
			registerCompletionRecursive(cmd, flagName, completionFunc)
		}
	}
}

// registerCompletionRecursive registers a flag completion function on all descendant commands.
// This is necessary because Cobra doesn't automatically propagate completion functions
// from parent to children for persistent flags.
func registerCompletionRecursive(cmd *cobra.Command, flagName string, completionFunc func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)) {
	for _, child := range cmd.Commands() {
		// Only register if the flag is accessible on this child command.
		if child.Flags().Lookup(flagName) != nil || child.PersistentFlags().Lookup(flagName) != nil {
			_ = child.RegisterFlagCompletionFunc(flagName, completionFunc)
		}
		// Recurse into grandchildren.
		registerCompletionRecursive(child, flagName, completionFunc)
	}
}

// BindToViper implements FlagParser.
// Binds both environment variables and Cobra pflags (if command is available) to Viper.
func (p *StandardFlagParser) BindToViper(v *viper.Viper) error {
	defer perf.Track(nil, "flags.StandardFlagParser.BindToViper")()

	// Store Viper instance for precedence handling in Parse()
	p.viper = v

	// Bind environment variables for each flag.
	// Do NOT bind pflags here - they should only be bound AFTER parsing in Parse().
	// Binding pflags before parsing causes the unparsed default values (often "")
	// to override SetDefault() values, breaking default handling.
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
// Sets defaults and binds both pflags and environment variables to ensure all
// precedence levels work correctly (CLI flags > ENV vars > defaults).
//
// Usage:
//
//	parser.RegisterFlags(cmd)
//	parser.BindFlagsToViper(cmd, viper.GetViper())
func (p *StandardFlagParser) BindFlagsToViper(cmd *cobra.Command, v *viper.Viper) error {
	defer perf.Track(nil, "flags.StandardFlagParser.BindFlagsToViper")()

	// Update the parser's viper instance so Parse() uses the correct viper.
	// This is critical when commands use viper.New() instead of viper.GetViper().
	p.viper = v

	// First, bind flags from this parser's registry.
	for _, flag := range p.registry.All() {
		viperKey := p.getViperKey(flag.GetName())

		// Set default value first (needed when using viper.New() instead of global viper).
		if err := bindFlagToViper(v, viperKey, flag); err != nil {
			return err
		}

		// Then bind the Cobra pflag to Viper for CLI precedence.
		cobraFlag := cmd.Flags().Lookup(flag.GetName())
		if cobraFlag == nil {
			continue
		}

		if err := v.BindPFlag(viperKey, cobraFlag); err != nil {
			return fmt.Errorf("failed to bind flag %s to viper: %w", flag.GetName(), err)
		}
	}

	// Also bind inherited flags (persistent flags from parent commands like RootCmd).
	// These are global flags that all commands inherit but aren't in this parser's registry.
	// Use InheritedFlags() to get flags inherited from parent commands.
	// Only bind if not already bound by the registry above.
	cmd.InheritedFlags().VisitAll(func(flag *pflag.Flag) {
		// Skip if already in registry (avoid duplicate binding).
		if p.registry.Get(flag.Name) != nil {
			return
		}

		// Bind inherited flag to Viper using its name as the key.
		// Errors are ignored to match the behavior of the main loop above.
		_ = v.BindPFlag(flag.Name, flag)
	})

	return nil
}

// parseFlags manually parses args into a combined FlagSet and extracts positional/separated args.
// Returns the combined FlagSet for validation, or nil if no parsing occurred.
func (p *StandardFlagParser) parseFlags(args []string, result *ParsedConfig) (*pflag.FlagSet, error) {
	defer perf.Track(nil, "flags.StandardFlagParser.parseFlags")()

	// Early return: no command or no args.
	if p.cmd == nil || len(args) == 0 {
		result.PositionalArgs = args
		result.SeparatedArgs = []string{}
		return nil, nil
	}

	// Create combined FlagSet with both local and inherited persistent flags.
	combinedFlags := p.createCombinedFlagSet()

	// Store combinedFlags for access by other code.
	p.parsedFlags = combinedFlags

	// Parse args with the combined FlagSet.
	if err := combinedFlags.Parse(args); err != nil {
		return nil, fmt.Errorf("failed to parse flags: %w", err)
	}

	// Extract positional and separated args.
	p.extractArgs(combinedFlags, result)

	// Bind changed flags to Viper.
	p.bindChangedFlagsToViper(combinedFlags)

	return combinedFlags, nil
}

// createCombinedFlagSet creates a FlagSet containing both local and inherited persistent flags.
func (p *StandardFlagParser) createCombinedFlagSet() *pflag.FlagSet {
	defer perf.Track(nil, "flags.StandardFlagParser.createCombinedFlagSet")()

	combinedFlags := pflag.NewFlagSet("combined", pflag.ContinueOnError)

	// Add inherited flags first (persistent flags from parent commands).
	p.cmd.InheritedFlags().VisitAll(func(flag *pflag.Flag) {
		combinedFlags.AddFlag(flag)
	})

	// Add local flags, skipping duplicates to avoid "flag redefined" panics.
	p.cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if combinedFlags.Lookup(flag.Name) == nil {
			combinedFlags.AddFlag(flag)
		}
	})

	return combinedFlags
}

// extractArgs splits args into positional args and separated args.
// Uses ArgsLenAtDash() to determine if "--" separator was present:
// - If "--" present: args before it are positional, args after are separated.
// - If no "--": all args are positional, separated is empty (let validation catch surplus args).
func (p *StandardFlagParser) extractArgs(flags *pflag.FlagSet, result *ParsedConfig) {
	defer perf.Track(nil, "flags.StandardFlagParser.extractArgs")()

	allArgs := flags.Args()
	dashIndex := flags.ArgsLenAtDash()

	// Check if "--" separator was present in args.
	if dashIndex >= 0 {
		// "--" was present: split at that point.
		// Args before "--" are positional, args after "--" are separated (pass-through).
		result.PositionalArgs = allArgs[:dashIndex]
		result.SeparatedArgs = allArgs[dashIndex:]
	} else {
		// No "--" separator: all args are positional, none are pass-through.
		// Let positional args validator catch any surplus args.
		result.PositionalArgs = allArgs
		result.SeparatedArgs = []string{}
	}
}

// bindChangedFlagsToViper binds flags that were changed during parsing to Viper.
func (p *StandardFlagParser) bindChangedFlagsToViper(combinedFlags *pflag.FlagSet) {
	defer perf.Track(nil, "flags.StandardFlagParser.bindChangedFlagsToViper")()

	if p.viper == nil {
		return
	}

	for _, flag := range p.registry.All() {
		viperKey := p.getViperKey(flag.GetName())
		cobraFlag := combinedFlags.Lookup(flag.GetName())
		if cobraFlag != nil && cobraFlag.Changed {
			// Only bind if the flag was actually provided on CLI.
			_ = p.viper.BindPFlag(viperKey, cobraFlag)
		}
	}
}

// validatePositionalArgs validates positional args using the configured validator.
// This is called after interactive prompts have had a chance to fill in missing values.
// Wraps validator errors with ErrInvalidPositionalArgs for consistent error handling.
func (p *StandardFlagParser) validatePositionalArgs(positionalArgs []string) error {
	defer perf.Track(nil, "flags.StandardFlagParser.validatePositionalArgs")()

	if p.positionalArgs != nil && p.positionalArgs.validator != nil {
		if err := p.positionalArgs.validator(p.cmd, positionalArgs); err != nil {
			// Wrap both errors for consistent error handling - allows errors.Is() to match either.
			return fmt.Errorf("%w: %w", errUtils.ErrInvalidPositionalArgs, err)
		}
	}
	return nil
}

// populateFlagsFromViper populates the Flags map from Viper with type conversion and default handling.
func (p *StandardFlagParser) populateFlagsFromViper(result *ParsedConfig, combinedFlags *pflag.FlagSet) {
	defer perf.Track(nil, "flags.StandardFlagParser.populateFlagsFromViper")()

	if p.viper == nil {
		return
	}

	for _, flag := range p.registry.All() {
		flagName := flag.GetName()
		viperKey := p.getViperKey(flagName)

		switch f := flag.(type) {
		case *BoolFlag:
			result.Flags[flagName] = p.viper.GetBool(viperKey)
		case *IntFlag:
			result.Flags[flagName] = p.viper.GetInt(viperKey)
		case *StringFlag:
			value := p.getStringFlagValue(f, flagName, viperKey, combinedFlags)
			result.Flags[flagName] = value
		case *StringSliceFlag:
			result.Flags[flagName] = p.viper.GetStringSlice(viperKey)
		default:
			result.Flags[flagName] = p.viper.Get(viperKey)
		}
	}
}

// getStringFlagValue gets a string flag value from Viper with proper default handling.
func (p *StandardFlagParser) getStringFlagValue(f *StringFlag, flagName, viperKey string, combinedFlags *pflag.FlagSet) string {
	defer perf.Track(nil, "flags.StandardFlagParser.getStringFlagValue")()

	value := p.viper.GetString(viperKey)

	// If Viper returns empty string and flag wasn't explicitly changed, use the flag's default.
	if value == "" {
		if combinedFlags == nil {
			return f.Default
		}
		cobraFlag := combinedFlags.Lookup(flagName)
		if cobraFlag != nil && !cobraFlag.Changed {
			return f.Default
		}
	}

	return value
}

// Parse implements FlagParser.
func (p *StandardFlagParser) Parse(ctx context.Context, args []string) (*ParsedConfig, error) {
	defer perf.Track(nil, "flags.StandardFlagParser.Parse")()

	result := &ParsedConfig{
		Flags:          make(map[string]interface{}),
		PositionalArgs: []string{},
		SeparatedArgs:  []string{},
	}

	// Step 1: Parse flags and extract positional/separated args.
	combinedFlags, err := p.parseFlags(args, result)
	if err != nil {
		return nil, err
	}

	// Step 2: Populate Flags map from Viper with precedence applied.
	p.populateFlagsFromViper(result, combinedFlags)

	// Step 3: Handle interactive prompts (all 3 use cases).
	// This must happen before positional arg validation because prompts may fill in missing args.
	if err := p.handleInteractivePrompts(result, combinedFlags); err != nil {
		return nil, err
	}

	// Step 4: Validate positional args (after prompts have filled in missing values).
	if err := p.validatePositionalArgs(result.PositionalArgs); err != nil {
		return nil, err
	}

	// Step 5: Validate flag values against valid values constraints.
	if combinedFlags != nil {
		if err := p.validateFlagValues(result.Flags, combinedFlags); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// validateFlagValues validates flag values against configured valid values constraints.
// Returns error if any flag value is not in its valid values list.
// Only validates flags that were explicitly changed by the user to avoid pollution from
// Viper/environment variables in tests where commands run sequentially.
// CombinedFlags is the FlagSet used for parsing (includes both local and inherited flags).
func (p *StandardFlagParser) validateFlagValues(flags map[string]interface{}, combinedFlags *pflag.FlagSet) error {
	defer perf.Track(nil, "flags.StandardFlagParser.validateFlagValues")()

	if p.validValues == nil {
		return nil
	}

	for flagName, validValues := range p.validValues {
		if err := p.validateSingleFlag(flagName, validValues, flags, combinedFlags); err != nil {
			return err
		}
	}

	return nil
}

// validateSingleFlag validates a single flag's value against its valid values list.
func (p *StandardFlagParser) validateSingleFlag(flagName string, validValues []string, flags map[string]interface{}, combinedFlags *pflag.FlagSet) error {
	defer perf.Track(nil, "flags.StandardFlagParser.validateSingleFlag")()

	value, exists := flags[flagName]
	if !exists {
		return nil
	}

	// Skip validation for flags not explicitly changed.
	if !p.isFlagExplicitlyChanged(flagName, combinedFlags) {
		return nil
	}

	// Convert to string and validate.
	strValue, ok := value.(string)
	if !ok || strValue == "" {
		return nil
	}

	// Check if value is in valid values list.
	if !p.isValueValid(strValue, validValues) {
		return p.createValidationError(flagName, strValue, validValues)
	}

	return nil
}

// isFlagExplicitlyChanged checks if a flag was explicitly changed by the user.
func (p *StandardFlagParser) isFlagExplicitlyChanged(flagName string, combinedFlags *pflag.FlagSet) bool {
	defer perf.Track(nil, "flags.StandardFlagParser.isFlagExplicitlyChanged")()

	if combinedFlags == nil {
		return true
	}

	cobraFlag := combinedFlags.Lookup(flagName)
	return cobraFlag == nil || cobraFlag.Changed
}

// isValueValid checks if a value is in the list of valid values.
func (p *StandardFlagParser) isValueValid(value string, validValues []string) bool {
	defer perf.Track(nil, "flags.StandardFlagParser.isValueValid")()

	for _, validValue := range validValues {
		if value == validValue {
			return true
		}
	}
	return false
}

// createValidationError creates an error for an invalid flag value.
func (p *StandardFlagParser) createValidationError(flagName, value string, validValues []string) error {
	defer perf.Track(nil, "flags.StandardFlagParser.createValidationError")()

	// Check for custom error message.
	if msg, hasMsg := p.validationMsgs[flagName]; hasMsg {
		return fmt.Errorf("%w: %s", errUtils.ErrInvalidFlagValue, msg)
	}

	// Default error message.
	return fmt.Errorf("%w: invalid value %q for flag --%s (valid values: %s)",
		errUtils.ErrInvalidFlagValue, value, flagName, strings.Join(validValues, ", "))
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
	defer perf.Track(nil, "flags.StandardFlagParser.GetIdentityFromCmd")()

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

// handleInteractivePrompts handles all 3 interactive prompt use cases:
// 1. Missing required flags
// 2. Optional value flags (sentinel pattern)
// 3. Missing required positional arguments.
func (p *StandardFlagParser) handleInteractivePrompts(result *ParsedConfig, combinedFlags *pflag.FlagSet) error {
	defer perf.Track(nil, "flags.StandardFlagParser.handleInteractivePrompts")()

	// Use Case 2: Handle optional value flags (--flag without value triggers prompt).
	if err := p.promptForOptionalValueFlags(result, combinedFlags); err != nil {
		return err
	}

	// Use Case 1: Handle missing required flags.
	if err := p.promptForMissingRequiredFlags(result, combinedFlags); err != nil {
		return err
	}

	// Use Case 3: Handle missing required positional arguments.
	if err := p.promptForMissingPositionalArgs(result); err != nil {
		return err
	}

	return nil
}

// promptForOptionalValueFlags handles Use Case 2: Optional value flags.
// When a flag has NoOptDefVal=cfg.IdentityFlagSelectValue and the user provides the flag
// without a value, prompt for selection.
func (p *StandardFlagParser) promptForOptionalValueFlags(result *ParsedConfig, combinedFlags *pflag.FlagSet) error {
	defer perf.Track(nil, "flags.StandardFlagParser.promptForOptionalValueFlags")()

	if combinedFlags == nil || len(p.optionalValuePrompts) == 0 {
		return nil
	}

	// Sort flag names for deterministic prompt order.
	flagNames := make([]string, 0, len(p.optionalValuePrompts))
	for flagName := range p.optionalValuePrompts {
		flagNames = append(flagNames, flagName)
	}
	sort.Strings(flagNames)

	for _, flagName := range flagNames {
		promptConfig := p.optionalValuePrompts[flagName]
		// Get current flag value.
		flagValue, ok := result.Flags[flagName].(string)
		if !ok {
			continue
		}

		// Check if flag value is the sentinel (user wants interactive selection).
		if flagValue != cfg.IdentityFlagSelectValue {
			continue
		}

		// Prompt for value.
		selectedValue, err := PromptForOptionalValue(&OptionalValuePromptContext{
			FlagName:       flagName,
			FlagValue:      flagValue,
			PromptTitle:    promptConfig.PromptTitle,
			CompletionFunc: promptConfig.CompletionFunc,
			Cmd:            p.cmd,
			Args:           result.PositionalArgs,
		})
		if err != nil {
			// Prompt failed (user aborted, error occurred, etc.) - return the error.
			return err
		}

		if selectedValue == "" {
			// Not interactive or no options available - fall back to default.
			if f := combinedFlags.Lookup(flagName); f != nil {
				result.Flags[flagName] = f.DefValue
			} else {
				result.Flags[flagName] = ""
			}
			continue
		}

		// Update flag value with selection.
		result.Flags[flagName] = selectedValue
	}

	return nil
}

// promptForMissingRequiredFlags handles Use Case 1: Missing required flags.
// If a required flag is not set and has a prompt config, show interactive prompt.
func (p *StandardFlagParser) promptForMissingRequiredFlags(result *ParsedConfig, combinedFlags *pflag.FlagSet) error {
	defer perf.Track(nil, "flags.StandardFlagParser.promptForMissingRequiredFlags")()

	if combinedFlags == nil || len(p.flagPrompts) == 0 {
		return nil
	}

	// Sort flag names for deterministic prompt order.
	flagNames := make([]string, 0, len(p.flagPrompts))
	for flagName := range p.flagPrompts {
		flagNames = append(flagNames, flagName)
	}
	sort.Strings(flagNames)

	for _, flagName := range flagNames {
		if err := p.promptForSingleMissingFlag(flagName, result, combinedFlags); err != nil {
			return err
		}
	}

	return nil
}

// promptForSingleMissingFlag prompts for a single missing required flag if needed.
func (p *StandardFlagParser) promptForSingleMissingFlag(flagName string, result *ParsedConfig, combinedFlags *pflag.FlagSet) error {
	defer perf.Track(nil, "flags.StandardFlagParser.promptForSingleMissingFlag")()

	promptConfig := p.flagPrompts[flagName]

	// Check if flag is missing (empty or not set).
	flagValue, ok := result.Flags[flagName].(string)
	if ok && flagValue != "" {
		return nil // Flag has value, no prompt needed.
	}

	// Check if flag was explicitly set to empty (user intentionally passed empty value).
	cobraFlag := combinedFlags.Lookup(flagName)
	if cobraFlag != nil && cobraFlag.Changed {
		return nil // User explicitly set the value (even if empty), don't prompt.
	}

	// Prompt for missing required flag.
	selectedValue, err := PromptForMissingRequired(
		flagName,
		promptConfig.PromptTitle,
		promptConfig.CompletionFunc,
		p.cmd,
		result.PositionalArgs,
	)
	if err != nil {
		return err
	}

	if selectedValue != "" {
		result.Flags[flagName] = selectedValue
	}

	return nil
}

// promptForMissingPositionalArgs handles Use Case 3: Missing required positional arguments.
// If a required positional arg is missing and has a prompt config, show interactive prompt.
func (p *StandardFlagParser) promptForMissingPositionalArgs(result *ParsedConfig) error {
	defer perf.Track(nil, "flags.StandardFlagParser.promptForMissingPositionalArgs")()

	if p.positionalArgs == nil || len(p.positionalPrompts) == 0 {
		return nil
	}

	// Iterate through positional arg specs in order.
	for i, spec := range p.positionalArgs.specs {
		// Check if this positional arg is missing.
		if i < len(result.PositionalArgs) {
			continue // Argument already provided.
		}

		// Check if this arg is required and has a prompt config.
		if !spec.Required {
			continue // Optional arg, no prompt needed.
		}

		promptConfig, hasPrompt := p.positionalPrompts[spec.Name]
		if !hasPrompt {
			continue // No prompt configured for this arg.
		}

		// Prompt for missing positional argument.
		selectedValue, err := PromptForPositionalArg(
			spec.Name,
			promptConfig.PromptTitle,
			promptConfig.CompletionFunc,
			p.cmd,
			result.PositionalArgs,
		)
		if err != nil {
			// Prompt failed (user aborted, error occurred, etc.) - return the error.
			return err
		}

		if selectedValue == "" {
			// Not interactive or no options available - skip this arg.
			// The command's validation will catch the missing required arg.
			continue
		}

		// Append the selected value to positional args.
		result.PositionalArgs = append(result.PositionalArgs, selectedValue)
	}

	return nil
}

// Reset clears any internal parser state to prevent pollution between test runs.
// This resets the command's flag state and the parsedFlags FlagSet.
func (p *StandardFlagParser) Reset() {
	defer perf.Track(nil, "flags.StandardFlagParser.Reset")()

	// Reset the command's flags if command is set.
	ResetCommandFlags(p.cmd)

	// Reset parsedFlags FlagSet if it was created.
	// This is specific to StandardFlagParser which maintains its own FlagSet.
	if p.parsedFlags != nil {
		p.parsedFlags.VisitAll(func(flag *pflag.Flag) {
			// Reset to default value.
			_ = flag.Value.Set(flag.DefValue)
			// Clear Changed state.
			flag.Changed = false
		})
	}
}
