package flags

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/perf"
)

// AtmosFlagParser is a unified parser that combines compatibility flag translation with standard Cobra flag parsing.
//
// Key features:
//   - Translates legacy flag syntax to Cobra-compatible format via compat.CompatibilityFlagTranslator.
//   - Enables Cobra validation (no DisableFlagParsing).
//   - Separates Atmos flags from separated args (for external tools).
//   - Handles double-dash separator (--) for explicit separation.
//   - Preprocesses NoOptDefVal flags to support space-separated syntax.
type AtmosFlagParser struct {
	cmd        *cobra.Command
	viper      *viper.Viper
	translator *compat.CompatibilityFlagTranslator
	registry   *FlagRegistry
}

// NewAtmosFlagParser creates a new unified parser.
func NewAtmosFlagParser(cmd *cobra.Command, v *viper.Viper, translator *compat.CompatibilityFlagTranslator, registry *FlagRegistry) *AtmosFlagParser {
	defer perf.Track(nil, "flags.NewAtmosFlagParser")()

	return &AtmosFlagParser{
		cmd:        cmd,
		viper:      v,
		translator: translator,
		registry:   registry,
	}
}

// Parse processes command-line arguments and returns parsed configuration.
//
// Process:
//  1. Set command on translator (enables shorthand normalization)
//  2. Validate no conflicts between compatibility flags and Cobra native shorthands
//  3. Split args at -- separator (if present)
//  4. Validate compatibility flag targets for aliases used in args
//  5. Translate compatibility flags + normalize Cobra shorthands in pre-separator args
//  6. Let Cobra parse the normalized Atmos args
//  7. Bind parsed flags to Viper
//  8. Collect separated args (post-separator + translated pass-through flags)
//  9. Handle NoOptDefVal resolution for empty flag values
//
// Example:
//
//	Input:  ["plan", "vpc", "-s", "dev", "-i=prod", "-var", "x=1", "--", "-var-file", "prod.tfvars"]
//	Step 3: argsBeforeSep=["plan", "vpc", "-s", "dev", "-i=prod", "-var", "x=1"], argsAfterSep=["-var-file", "prod.tfvars"]
//	Step 5: atmosArgs=["plan", "vpc", "-s", "dev", "--identity=prod"], separatedArgs=["-var", "x=1"]
//	        (-i=prod normalized to --identity=prod, -var moved to separated args)
//	Step 6: Cobra parses ["plan", "vpc", "-s", "dev", "--identity=prod"] (Cobra handles -s → --stack natively)
//	Result: Flags{stack="dev", identity="prod"}, Positional=["plan", "vpc"], SeparatedArgs=["-var", "x=1", "-var-file", "prod.tfvars"]
func (p *AtmosFlagParser) Parse(args []string) (*ParsedConfig, error) {
	defer perf.Track(nil, "flags.AtmosFlagParser.Parse")()

	// Step 1: Validate no conflicts with Cobra native shorthands.
	// This catches configuration errors like adding -s to compatibility flags
	// when it's already registered as a Cobra shorthand via StringP("stack", "s", ...).
	if err := p.translator.ValidateNoConflicts(p.cmd); err != nil {
		return nil, err
	}

	// Step 2: Split args at -- separator.
	argsBeforeSep, argsAfterSep := splitAtSeparator(args)

	// Step 2.5: Detect if Cobra has already parsed flags and adjust separation.
	argsBeforeSep, argsAfterSep = p.adjustForCobraParsing(argsBeforeSep, argsAfterSep)

	// Step 2.6: Preprocess NoOptDefVal flags (identity, pager).
	// Rewrite --flag value → --flag=value for flags with NoOptDefVal.
	// This maintains backward compatibility while working within Cobra's documented
	// limitation that NoOptDefVal requires equals syntax (pflag #134, #321, cobra #1962).
	// Only applies to args before the -- separator (Atmos flags, not pass-through).
	if p.registry != nil && len(argsBeforeSep) > 0 {
		argsBeforeSep = p.registry.PreprocessNoOptDefValArgs(argsBeforeSep)
	}

	// Step 3: Normalize Cobra shorthand flags with = syntax (e.g., -i=value → --identity=value).
	normalizedArgs := p.normalizeShorthandFlags(argsBeforeSep)

	// Step 4: Validate compatibility flag targets that are actually used in args.
	if err := p.translator.ValidateTargetsInArgs(p.cmd, normalizedArgs); err != nil {
		return nil, err
	}

	// Step 5: Translate compatibility flags in normalized args.
	// This handles terraform-specific flags like -var, -var-file, etc.
	atmosArgs, translatedSeparated := p.translator.Translate(normalizedArgs)

	// Step 6: Parse and bind flags to Viper.
	if err := p.parseAndBindFlags(atmosArgs); err != nil {
		return nil, err
	}

	// Step 7: Collect separated args (translated pass-through + args after --).
	separatedArgs := p.collectSeparatedArgs(translatedSeparated, argsAfterSep)

	// Step 8: Extract positional args (non-flag args parsed by Cobra).
	positionalArgs := p.cmd.Flags().Args()

	// Step 9: Handle NoOptDefVal resolution for empty flag values.
	p.resolveNoOptDefValForEmptyFlags()

	// Step 10: Build flags map from Viper.
	flagsMap := p.buildFlagsMap()

	return &ParsedConfig{
		Flags:          flagsMap,
		SeparatedArgs:  separatedArgs,
		PositionalArgs: positionalArgs,
	}, nil
}

// splitAtSeparator splits args at the -- separator.
// Returns (argsBeforeSep, argsAfterSep).
func splitAtSeparator(args []string) ([]string, []string) {
	for i, arg := range args {
		if arg == "--" {
			return args[:i], args[i+1:]
		}
	}
	return args, []string{}
}

// resolveNoOptDefValForEmptyFlags handles flags that have empty string values.
//
// For flags that support optional values (like --identity for interactive selection),
// we treat empty string as a signal to use a special marker value ("__SELECT__").
//
// Example:
//   - --identity=prod  → identity="prod"
//   - --identity=      → identity="__SELECT__" (triggers interactive selection)
//
// This works around Cobra's NoOptDefVal limitation where --identity value treats
// "value" as a positional arg instead of the flag value.
func (p *AtmosFlagParser) resolveNoOptDefValForEmptyFlags() {
	defer perf.Track(nil, "flags.FlagParser.resolveNoOptDefValForEmptyFlags")()

	// Hard-coded list of flags that support empty-value interactive selection.
	// In future, this could be configurable via builder pattern.
	interactiveFlags := map[string]string{
		"identity": "__SELECT__",
	}

	p.cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		// Check if this flag supports interactive selection.
		marker, ok := interactiveFlags[flag.Name]
		if !ok {
			return
		}

		// Check if flag was set to empty string.
		if flag.Changed && flag.Value.String() == "" {
			// Replace empty value with selection marker.
			_ = flag.Value.Set(marker)
			// Update Viper as well.
			p.viper.Set(flag.Name, marker)
		}
	})
}

// Reset clears any internal parser state to prevent pollution between test runs.
// This resets the command's flag state to prevent flag values from one test
// polluting subsequent tests when using a global parser instance.
func (p *AtmosFlagParser) Reset() {
	defer perf.Track(nil, "flags.FlagParser.Reset")()

	ResetCommandFlags(p.cmd)
}

// GetArgsForTool builds the complete argument array for executing a subprocess tool.
// This eliminates manual args building boilerplate throughout the codebase.
//
// Format: [subcommand, component, ...pass-through-args]
// Example: ["plan", "vpc", "-var-file", "common.tfvars"]
//
// Usage:
//
//	args := result.GetArgsForTool()  // Instead of manual: append(result.PositionalArgs, result.SeparatedArgs...)
func (pc *ParsedConfig) GetArgsForTool() []string {
	defer perf.Track(nil, "flags.ParsedConfig.GetArgsForTool")()

	args := make([]string, 0, len(pc.PositionalArgs)+len(pc.SeparatedArgs))
	args = append(args, pc.PositionalArgs...)
	args = append(args, pc.SeparatedArgs...)
	return args
}

// NormalizeShorthandWithEquals normalizes shorthand flags with = syntax to longhand format.
// This fixes a Cobra quirk where -i=value works but -i= returns literal "=" instead of empty string.
//
// Examples:
//   - -i=value → --identity=value
//   - -i= → --identity=
//   - -s=dev → --stack=dev
//
// This ensures consistent behavior: -i=value behaves the same as --identity=value.
//
// Returns:
//   - normalized: The normalized flag (e.g., "--identity=value")
//   - wasNormalized: True if normalization occurred, false otherwise
func NormalizeShorthandWithEquals(cmd *cobra.Command, arg string) (normalized string, wasNormalized bool) {
	// Only process single-dash flags with = syntax.
	if !strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "--") {
		return arg, false
	}

	// Check if arg has = syntax.
	idx := strings.Index(arg, "=")
	if idx <= 0 {
		return arg, false
	}

	// Extract shorthand (e.g., "-i=" → "i").
	shorthand := arg[1:idx]
	valuePart := arg[idx:] // "=value" or just "="

	// Look up the longhand flag name for this shorthand.
	var longhand string
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if flag.Shorthand == shorthand {
			longhand = flag.Name
		}
	})

	// If no longhand found, this is not a registered Cobra shorthand.
	// Return original arg unchanged.
	if longhand == "" {
		return arg, false
	}

	// Normalize to longhand format: -i=value → --identity=value.
	normalized = "--" + longhand + valuePart
	return normalized, true
}

// adjustForCobraParsing detects if Cobra has already parsed flags and adjusts arg separation.
// If there's no -- separator in args AND Cobra has already parsed flags (detected by
// checking if any flags are marked Changed), then ALL args should be treated as
// separated args (the command to execute).
// This handles the case where: user runs "atmos auth exec --identity=prod -- sh -c echo"
// -> Cobra parses --identity=prod and strips -- -> RunE receives ["sh", "-c", "echo"]
// -> We need to treat these as separated args, not as flags to parse.
func (p *AtmosFlagParser) adjustForCobraParsing(argsBeforeSep, argsAfterSep []string) ([]string, []string) {
	defer perf.Track(nil, "flags.FlagParser.adjustForCobraParsing")()

	if len(argsAfterSep) == 0 && len(argsBeforeSep) > 0 {
		// No -- separator found, check if Cobra already parsed flags.
		if p.hasCobraAlreadyParsedFlags() {
			// Cobra already parsed flags and removed --, so all remaining args are separated args.
			return []string{}, argsBeforeSep
		}
	}

	return argsBeforeSep, argsAfterSep
}

// hasCobraAlreadyParsedFlags checks if any flags have been marked as Changed by Cobra.
func (p *AtmosFlagParser) hasCobraAlreadyParsedFlags() bool {
	defer perf.Track(nil, "flags.FlagParser.hasCobraAlreadyParsedFlags")()

	cobraAlreadyParsed := false
	p.cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Changed {
			cobraAlreadyParsed = true
		}
	})

	return cobraAlreadyParsed
}

// normalizeShorthandFlags normalizes Cobra shorthand flags with = syntax.
// This fixes a Cobra quirk where -i= returns literal "=" instead of empty string.
// This happens BEFORE compatibility flag translation because it's about native Cobra flags.
func (p *AtmosFlagParser) normalizeShorthandFlags(argsBeforeSep []string) []string {
	defer perf.Track(nil, "flags.FlagParser.normalizeShorthandFlags")()

	normalizedArgs := make([]string, len(argsBeforeSep))
	for i, arg := range argsBeforeSep {
		if normalized, wasNormalized := NormalizeShorthandWithEquals(p.cmd, arg); wasNormalized {
			normalizedArgs[i] = normalized
		} else {
			normalizedArgs[i] = arg
		}
	}

	return normalizedArgs
}

// parseAndBindFlags parses Cobra flags and binds them to Viper.
func (p *AtmosFlagParser) parseAndBindFlags(atmosArgs []string) error {
	defer perf.Track(nil, "flags.FlagParser.parseAndBindFlags")()

	// Let Cobra parse the normalized Atmos args.
	// We need to temporarily set the args on the command for Cobra to parse them.
	p.cmd.SetArgs(atmosArgs)

	// Execute Cobra parsing (this populates cmd.Flags()).
	if err := p.cmd.ParseFlags(atmosArgs); err != nil {
		return err
	}

	// Bind parsed flags to Viper.
	if err := p.viper.BindPFlags(p.cmd.Flags()); err != nil {
		return err
	}

	return nil
}

// collectSeparatedArgs combines translated pass-through args and args after --.
func (p *AtmosFlagParser) collectSeparatedArgs(translatedSeparated, argsAfterSep []string) []string {
	defer perf.Track(nil, "flags.FlagParser.collectSeparatedArgs")()

	separatedArgs := make([]string, 0, len(translatedSeparated)+len(argsAfterSep))
	separatedArgs = append(separatedArgs, translatedSeparated...)
	separatedArgs = append(separatedArgs, argsAfterSep...)

	return separatedArgs
}

// buildFlagsMap builds a flags map from Viper for backward compatibility.
func (p *AtmosFlagParser) buildFlagsMap() map[string]interface{} {
	defer perf.Track(nil, "flags.FlagParser.buildFlagsMap")()

	flagsMap := make(map[string]interface{})
	for key := range p.viper.AllSettings() {
		flagsMap[key] = p.viper.Get(key)
	}

	return flagsMap
}
