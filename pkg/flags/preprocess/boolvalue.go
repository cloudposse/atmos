package preprocess

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/terminal/env"
)

// BoolFlagInfo is an optional interface a FlagInfo may implement to declare
// that it is a boolean flag. Flags that do not implement it are not boolean.
type BoolFlagInfo interface {
	// IsBool reports whether the flag is boolean.
	IsBool() bool
}

// BoolValuePreprocessor rewrites "--flag true|false" to "--flag=true|false" for
// registered boolean flags.
//
// Cobra boolean flags never consume the following argument, so without this
// step "--no-color false" leaves "false" behind as a stray positional argument
// (an unknown command, or an argument forwarded to Terraform).
// Only the exact literals "true" and "false" (case-insensitive) are folded into
// the flag. Any other following argument, such as a subcommand or component
// name, is left alone so a bare "--flag" keeps meaning "true".
//
// Example transformation:
//
//	Input:  ["--logs-color", "false", "version"]
//	Output: ["--logs-color=false", "version"]
//
// Arguments after the "--" terminator are never modified.
type BoolValuePreprocessor struct {
	boolFlags map[string]bool
}

// NewBoolValuePreprocessor creates a preprocessor for the boolean flags among the given flags.
// Both the long name and the shorthand of each boolean flag are recognized.
func NewBoolValuePreprocessor(flags []FlagInfo) *BoolValuePreprocessor {
	defer perf.Track(nil, "preprocess.NewBoolValuePreprocessor")()

	boolFlags := make(map[string]bool)
	for _, flag := range flags {
		b, ok := flag.(BoolFlagInfo)
		if !ok || !b.IsBool() {
			continue
		}
		if name := flag.GetName(); name != "" {
			boolFlags[name] = true
		}
		if shorthand := flag.GetShorthand(); shorthand != "" {
			boolFlags[shorthand] = true
		}
	}
	return &BoolValuePreprocessor{boolFlags: boolFlags}
}

// Preprocess rewrites "--flag true|false" to "--flag=true|false" for boolean flags.
func (p *BoolValuePreprocessor) Preprocess(args []string) []string {
	defer perf.Track(nil, "preprocess.BoolValuePreprocessor.Preprocess")()

	if len(p.boolFlags) == 0 {
		return args
	}
	return env.NormalizeBoolFlagValues(args, func(name string) bool {
		return p.boolFlags[name]
	})
}
