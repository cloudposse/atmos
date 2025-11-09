package compat

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// CompatibilityBehavior defines how a compatibility flag should be handled.
type CompatibilityBehavior int

const (
	// MapToAtmosFlag converts the legacy flag to a modern Atmos flag.
	// Example: -s → --stack, -var → --var.
	MapToAtmosFlag CompatibilityBehavior = iota

	// AppendToSeparated appends the flag and its value to separated args.
	// Example: -var-file → append to separated args for pass-through to terraform.
	AppendToSeparated
)

const (
	// FlagPrefix is the single dash prefix for flags.
	flagPrefix = "-"
)

// CompatibilityFlag defines how a single compatibility flag should be handled.
type CompatibilityFlag struct {
	Behavior CompatibilityBehavior
	Target   string // Target flag name (for MapToAtmosFlag) or empty (for AppendToSeparated)
}

// CompatibilityFlagTranslator translates legacy flag syntax to modern Cobra-compatible format.
// It separates args into two categories:
//   - atmosArgs: Args that should be parsed by Cobra (Atmos flags)
//   - separatedArgs: Args that should be passed through to subprocess (terraform, etc.)
type CompatibilityFlagTranslator struct {
	flagMap map[string]CompatibilityFlag
}

// NewCompatibilityFlagTranslator creates a new translator with the given flag map.
func NewCompatibilityFlagTranslator(flagMap map[string]CompatibilityFlag) *CompatibilityFlagTranslator {
	defer perf.Track(nil, "flags.NewCompatibilityFlagTranslator")()

	return &CompatibilityFlagTranslator{
		flagMap: flagMap,
	}
}

// ValidateTargets validates that all compatibility flags with MapToAtmosFlag behavior
// reference flags that are actually registered on the command.
// This should be called after flags are registered but before parsing.
func (t *CompatibilityFlagTranslator) ValidateTargets(cmd *cobra.Command) error {
	defer perf.Track(nil, "flags.CompatibilityFlagTranslator.ValidateTargets")()

	for compatFlag, config := range t.flagMap {
		// Only validate MapToAtmosFlag flags (those with Target set).
		if config.Behavior != MapToAtmosFlag || config.Target == "" {
			continue
		}

		// Remove leading dashes from target to get flag name.
		targetFlagName := strings.TrimLeft(config.Target, flagPrefix)

		// Check if flag exists in command.
		flag := cmd.Flags().Lookup(targetFlagName)
		if flag == nil {
			return fmt.Errorf("%w: compatibility flag %q references non-existent flag %q", errUtils.ErrCompatibilityFlagMissingTarget, compatFlag, config.Target)
		}
	}

	return nil
}

// ValidateNoConflicts validates that compatibility flags don't conflict with Cobra native shorthands.
// This detects configuration errors where someone adds -s or -i to compatibility flags
// when they're already registered as Cobra shorthands via StringP("stack", "s", ...).
// This should be called after flags are registered.
func (t *CompatibilityFlagTranslator) ValidateNoConflicts(cmd *cobra.Command) error {
	defer perf.Track(nil, "flags.CompatibilityFlagTranslator.ValidateNoConflicts")()

	for compatFlag := range t.flagMap {
		// Only check single-dash flags (potential shorthands).
		if !strings.HasPrefix(compatFlag, flagPrefix) || strings.HasPrefix(compatFlag, "--") {
			continue
		}

		// Extract shorthand (e.g., "-s" → "s").
		shorthand := strings.TrimPrefix(compatFlag, flagPrefix)

		// Check if this shorthand is already registered as a Cobra flag shorthand.
		cmd.Flags().VisitAll(func(flag *pflag.Flag) {
			if flag.Shorthand == shorthand {
				// This is an error - the compatibility flag conflicts with a Cobra native shorthand.
				panic(fmt.Sprintf(
					"compatibility flag %q conflicts with Cobra native shorthand for flag %q. "+
						"Remove %q from compatibility flags - Cobra handles it automatically via StringP(%q, %q, ...)",
					compatFlag, flag.Name, compatFlag, flag.Name, shorthand,
				))
			}
		})
	}

	return nil
}

// ValidateTargetsInArgs validates that compatibility flags used in the given args
// reference flags that are actually registered on the command.
// This only validates compatibility flags that appear in the args, not all registered flags.
func (t *CompatibilityFlagTranslator) ValidateTargetsInArgs(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "flags.CompatibilityFlagTranslator.ValidateTargetsInArgs")()

	// Extract compatibility flags used in args.
	usedFlags := make(map[string]bool)
	for _, arg := range args {
		// Skip non-flags.
		if !strings.HasPrefix(arg, flagPrefix) {
			continue
		}

		// Skip already-modern flags (--).
		if strings.HasPrefix(arg, "--") {
			continue
		}

		// Extract flag name (handle -flag=value form).
		flagName := arg
		if idx := strings.Index(arg, "="); idx > 0 {
			flagName = arg[:idx]
		}

		usedFlags[flagName] = true
	}

	// Validate only the used compatibility flags.
	for compatFlag := range usedFlags {
		config, ok := t.flagMap[compatFlag]
		if !ok {
			// Unknown flag - not our concern, Cobra will handle it.
			continue
		}

		// Only validate MapToAtmosFlag flags (those with Target set).
		if config.Behavior != MapToAtmosFlag || config.Target == "" {
			continue
		}

		// Remove leading dashes from target to get flag name.
		targetFlagName := strings.TrimLeft(config.Target, flagPrefix)

		// Check if flag exists in command.
		flag := cmd.Flags().Lookup(targetFlagName)
		if flag == nil {
			return fmt.Errorf("%w: compatibility flag %q references non-existent flag %q", errUtils.ErrCompatibilityFlagMissingTarget, compatFlag, config.Target)
		}
	}

	return nil
}

// Translate processes args and separates them into Atmos args and separated args.
// Returns:
//   - atmosArgs: Arguments for Cobra to parse (Atmos flags + positional args)
//   - separatedArgs: Arguments to pass through to subprocess
func (t *CompatibilityFlagTranslator) Translate(args []string) (atmosArgs []string, separatedArgs []string) {
	defer perf.Track(nil, "flags.CompatibilityFlagTranslator.Translate")()

	atmosArgs = make([]string, 0, len(args))
	separatedArgs = make([]string, 0)

	for i := 0; i < len(args); i++ {
		arg := args[i]

		// Not a flag - it's a positional arg
		if !strings.HasPrefix(arg, flagPrefix) {
			atmosArgs = append(atmosArgs, arg)
			continue
		}

		// Already modern format (--flag) - pass to Atmos
		if strings.HasPrefix(arg, "--") {
			atmosArgs = append(atmosArgs, arg)
			continue
		}

		// Single-dash flag - check for compatibility flag
		translated, consumed := t.translateSingleDashFlag(args, i)

		// Add translated args to appropriate destination
		atmosArgs = append(atmosArgs, translated.atmosArgs...)
		separatedArgs = append(separatedArgs, translated.separatedArgs...)

		// Skip consumed args
		i += consumed
	}

	return atmosArgs, separatedArgs
}

// translatedArgs holds the result of translating a single flag.
type translatedArgs struct {
	atmosArgs     []string
	separatedArgs []string
}

// translateSingleDashFlag translates a single-dash flag based on compatibility flag rules.
// Returns the translated args and the number of additional args consumed.
//
// NOTE: Cobra shorthand normalization (-i=value → --identity=value) happens BEFORE this method is called.
// This method only handles compatibility flags for terraform-specific flags like -var, -var-file, etc.
func (t *CompatibilityFlagTranslator) translateSingleDashFlag(args []string, index int) (translatedArgs, int) {
	arg := args[index]

	// Check for -flag=value form (compatibility flags).
	if idx := strings.Index(arg, "="); idx > 0 {
		return t.translateFlagWithEquals(arg, idx)
	}

	// Check for -flag form (value might be next arg).
	return t.translateFlagWithoutEquals(args, index, arg)
}

// translateFlagWithEquals handles -flag=value syntax for compatibility flags.
func (t *CompatibilityFlagTranslator) translateFlagWithEquals(arg string, equalsIndex int) (translatedArgs, int) {
	defer perf.Track(nil, "flags.CompatibilityFlagTranslator.translateFlagWithEquals")()

	flagPart := arg[:equalsIndex]  // "-var"
	valuePart := arg[equalsIndex:] // "=foo=bar"

	if compatFlag, ok := t.flagMap[flagPart]; ok {
		return t.applyFlagBehaviorWithEquals(compatFlag, arg, valuePart)
	}

	// Unknown flag with = - pass to Atmos (Cobra will validate).
	return translatedArgs{
		atmosArgs:     []string{arg},
		separatedArgs: []string{},
	}, 0
}

// applyFlagBehaviorWithEquals applies the flag behavior for -flag=value syntax.
func (t *CompatibilityFlagTranslator) applyFlagBehaviorWithEquals(compatFlag CompatibilityFlag, arg string, valuePart string) (translatedArgs, int) {
	defer perf.Track(nil, "flags.CompatibilityFlagTranslator.applyFlagBehaviorWithEquals")()

	switch compatFlag.Behavior {
	case MapToAtmosFlag:
		// Convert: -var=foo=bar → --var=foo=bar
		return translatedArgs{
			atmosArgs:     []string{compatFlag.Target + valuePart},
			separatedArgs: []string{},
		}, 0

	case AppendToSeparated:
		// Append to separated: -var-file=prod.tfvars → separated args
		return translatedArgs{
			atmosArgs:     []string{},
			separatedArgs: []string{arg}, // Keep original format
		}, 0

	default:
		// Unknown behavior - pass to Atmos.
		return translatedArgs{
			atmosArgs:     []string{arg},
			separatedArgs: []string{},
		}, 0
	}
}

// translateFlagWithoutEquals handles -flag syntax where value might be in next arg.
func (t *CompatibilityFlagTranslator) translateFlagWithoutEquals(args []string, index int, arg string) (translatedArgs, int) {
	defer perf.Track(nil, "flags.CompatibilityFlagTranslator.translateFlagWithoutEquals")()

	if compatFlag, ok := t.flagMap[arg]; ok {
		return t.applyFlagBehaviorWithoutEquals(compatFlag, args, index, arg)
	}

	// Unknown single-dash flag - pass to Atmos (Cobra will error if truly unknown).
	// This handles valid Atmos shorthands that aren't in the flag map.
	return t.handleUnknownSingleDashFlag(args, index, arg)
}

// applyFlagBehaviorWithoutEquals applies the flag behavior for -flag syntax.
func (t *CompatibilityFlagTranslator) applyFlagBehaviorWithoutEquals(compatFlag CompatibilityFlag, args []string, index int, arg string) (translatedArgs, int) {
	defer perf.Track(nil, "flags.CompatibilityFlagTranslator.applyFlagBehaviorWithoutEquals")()

	switch compatFlag.Behavior {
	case MapToAtmosFlag:
		// Convert: -s dev → --stack dev
		translated, consumed := t.extractFlagWithValue(args, index, compatFlag.Target)
		return translatedArgs{
			atmosArgs:     translated,
			separatedArgs: []string{},
		}, consumed

	case AppendToSeparated:
		// Append to separated: -var-file prod.tfvars → separated args
		moved, consumed := t.extractFlagWithValue(args, index, arg)
		return translatedArgs{
			atmosArgs:     []string{},
			separatedArgs: moved,
		}, consumed

	default:
		// Unknown behavior - pass to Atmos.
		return translatedArgs{
			atmosArgs:     []string{arg},
			separatedArgs: []string{},
		}, 0
	}
}

// extractFlagWithValue extracts a flag and its value (if present in next arg).
func (t *CompatibilityFlagTranslator) extractFlagWithValue(args []string, index int, flag string) ([]string, int) {
	defer perf.Track(nil, "flags.CompatibilityFlagTranslator.extractFlagWithValue")()

	result := []string{flag}
	consumed := 0

	// Check if next arg is the value (not another flag).
	if index+1 < len(args) && !strings.HasPrefix(args[index+1], flagPrefix) {
		result = append(result, args[index+1])
		consumed = 1 // Consume the value arg
	}

	return result, consumed
}

// handleUnknownSingleDashFlag handles unknown single-dash flags (pass to Atmos).
func (t *CompatibilityFlagTranslator) handleUnknownSingleDashFlag(args []string, index int, arg string) (translatedArgs, int) {
	defer perf.Track(nil, "flags.CompatibilityFlagTranslator.handleUnknownSingleDashFlag")()

	result := []string{arg}
	consumed := 0

	// If there's a next arg that doesn't look like a flag, include it.
	// Cobra will handle whether it's a value or a positional arg.
	if index+1 < len(args) && !strings.HasPrefix(args[index+1], flagPrefix) {
		result = append(result, args[index+1])
		consumed = 1
	}

	return translatedArgs{
		atmosArgs:     result,
		separatedArgs: []string{},
	}, consumed
}
