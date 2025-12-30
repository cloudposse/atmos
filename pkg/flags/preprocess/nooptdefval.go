package preprocess

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// FlagInfo represents the information needed about a flag for preprocessing.
// This interface matches the methods needed from flags.Flag, avoiding circular imports.
type FlagInfo interface {
	// GetName returns the flag's name (e.g., "identity").
	GetName() string
	// GetShorthand returns the flag's shorthand (e.g., "i").
	GetShorthand() string
	// GetNoOptDefVal returns the flag's NoOptDefVal sentinel value.
	GetNoOptDefVal() string
}

// NoOptDefValPreprocessor rewrites space-separated syntax to equals syntax
// for flags with NoOptDefVal set.
//
// This works around a Cobra/pflag limitation where NoOptDefVal only works with
// equals syntax (--flag=value), not space-separated syntax (--flag value).
// See: https://github.com/spf13/pflag/issues/134, pflag#321, cobra#1962
//
// Example transformation:
//
//	Input:  ["--identity", "prod", "plan"]
//	Output: ["--identity=prod", "plan"]
//
// Without preprocessing, "prod" would be treated as a positional argument
// because Cobra's NoOptDefVal assumes the user wants the default value
// when no equals sign is present.
type NoOptDefValPreprocessor struct {
	flags []FlagInfo
}

// NewNoOptDefValPreprocessor creates a new preprocessor with the given flags.
// The flags slice should contain all flags that may have NoOptDefVal set.
func NewNoOptDefValPreprocessor(flags []FlagInfo) *NoOptDefValPreprocessor {
	defer perf.Track(nil, "preprocess.NewNoOptDefValPreprocessor")()

	return &NoOptDefValPreprocessor{
		flags: flags,
	}
}

// Preprocess rewrites space-separated flag syntax to equals syntax
// for flags that have NoOptDefVal set.
func (p *NoOptDefValPreprocessor) Preprocess(args []string) []string {
	defer perf.Track(nil, "preprocess.NoOptDefValPreprocessor.Preprocess")()

	if len(p.flags) == 0 {
		return args
	}

	noOptDefValFlags := p.buildNoOptDefValFlagsSet()

	// If no flags have NoOptDefVal, return args unchanged.
	if len(noOptDefValFlags) == 0 {
		return args
	}

	return p.preprocessArgs(args, noOptDefValFlags)
}

// buildNoOptDefValFlagsSet builds a set of flag names (long and short) that have NoOptDefVal.
func (p *NoOptDefValPreprocessor) buildNoOptDefValFlagsSet() map[string]bool {
	defer perf.Track(nil, "preprocess.NoOptDefValPreprocessor.buildNoOptDefValFlagsSet")()

	noOptDefValFlags := make(map[string]bool)
	for _, flag := range p.flags {
		if flag.GetNoOptDefVal() != "" {
			noOptDefValFlags[flag.GetName()] = true
			if shorthand := flag.GetShorthand(); shorthand != "" {
				noOptDefValFlags[shorthand] = true
			}
		}
	}
	return noOptDefValFlags
}

// preprocessArgs preprocesses args to rewrite space-separated syntax to equals syntax for NoOptDefVal flags.
func (p *NoOptDefValPreprocessor) preprocessArgs(args []string, noOptDefValFlags map[string]bool) []string {
	defer perf.Track(nil, "preprocess.NoOptDefValPreprocessor.preprocessArgs")()

	result := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]

		// Skip non-flag arguments.
		if !isFlagArg(arg) {
			result = append(result, arg)
			continue
		}

		// Process flag argument.
		processed, skip := p.processFlagArg(arg, args, i, noOptDefValFlags)
		result = append(result, processed)
		if skip {
			i++ // Skip the next arg (already consumed).
		}
	}

	return result
}

// processFlagArg processes a single flag argument and returns the processed arg and whether to skip next arg.
func (p *NoOptDefValPreprocessor) processFlagArg(arg string, args []string, i int, noOptDefValFlags map[string]bool) (string, bool) {
	defer perf.Track(nil, "preprocess.NoOptDefValPreprocessor.processFlagArg")()

	// Keep arg unchanged if it already has equals syntax.
	if hasSeparatedValue(arg) {
		return arg, false
	}

	// Extract flag name and check if it has NoOptDefVal.
	flagName := extractFlagName(arg)
	if !noOptDefValFlags[flagName] {
		return arg, false
	}

	// Check if there's a value following the flag.
	if !p.hasValueFollowing(args, i) {
		return arg, false
	}

	// Combine flag with following value using equals syntax.
	combined := arg + "=" + args[i+1]
	return combined, true
}

// hasValueFollowing checks if there's a value following the flag at position i.
func (p *NoOptDefValPreprocessor) hasValueFollowing(args []string, i int) bool {
	defer perf.Track(nil, "preprocess.NoOptDefValPreprocessor.hasValueFollowing")()

	return i+1 < len(args) && !isFlagArg(args[i+1])
}

// isFlagArg returns true if the arg looks like a flag (starts with - or --).
func isFlagArg(arg string) bool {
	return len(arg) > 0 && arg[0] == '-'
}

// hasSeparatedValue returns true if the flag already has equals syntax (--flag=value or -f=value).
// This checks if the argument contains an '=' character, indicating the value is attached to the flag.
func hasSeparatedValue(arg string) bool {
	return strings.Contains(arg, "=")
}

// extractFlagName extracts the flag name from --flag or -f.
// Examples: --identity → identity, -i → i, --stack → stack.
func extractFlagName(arg string) string {
	// Strip leading dashes.
	name := arg
	for len(name) > 0 && name[0] == '-' {
		name = name[1:]
	}
	return name
}
