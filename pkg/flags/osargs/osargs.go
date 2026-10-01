// Package osargs parses individual flag values directly from raw os.Args,
// before Cobra or Viper have processed them. It exists for the small set of
// callers that need a flag value before normal config loading (e.g. profile
// or edition pins consulted while building the Viper instance) or as a
// fallback for commands that run with Cobra's DisableFlagParsing=true
// (terraform, helmfile, packer, auth exec), where Cobra never populates a
// flag binding at all.
package osargs

import (
	"io"

	"github.com/spf13/pflag"

	"github.com/cloudposse/atmos/pkg/perf"
)

// newFlagSet builds a throwaway pflag.FlagSet for extracting a single flag's
// value from raw args, suppressing all the side effects a real command's
// FlagSet would have: no usage/error output, unknown flags ignored.
//
// Usage suppression matters because callers may parse the same args more than
// once during a single command's lifecycle (e.g. once in Execute() and again
// in PersistentPreRun) — without it, `--help`/`-h` would print a duplicate
// "Usage of <name>:" block, since pflag implicitly handles those flags and
// calls fs.Usage() before returning ErrHelp.
func newFlagSet(name string) *pflag.FlagSet {
	fs := pflag.NewFlagSet(name, pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.ParseErrorsAllowlist.UnknownFlags = true
	fs.Usage = func() {}
	return fs
}

// ParseString extracts a single string flag's value from raw args, or ""
// if the flag isn't present. Whitespace is trimmed by the caller if needed —
// this returns the raw pflag-parsed value.
func ParseString(args []string, flagName string) string {
	defer perf.Track(nil, "osargs.ParseString")()

	fs := newFlagSet(flagName + "-parser")
	value := fs.String(flagName, "", "")
	_ = fs.Parse(args) // Ignore errors from unknown flags.
	return *value
}

// ParseStringWithShorthand extracts a single string flag's value from raw
// args, matching either its long form (--flagName) or single-character
// shorthand (-X), or "" if neither is present. Uses pflag's own shorthand
// support, so it recognizes every syntax pflag does: --flag=value,
// --flag value, -X=value, -Xvalue (concatenated), and -X value.
func ParseStringWithShorthand(args []string, flagName, shorthand string) string {
	defer perf.Track(nil, "osargs.ParseStringWithShorthand")()

	fs := newFlagSet(flagName + "-parser")
	value := fs.StringP(flagName, shorthand, "", "")
	_ = fs.Parse(args) // Ignore errors from unknown flags.
	return *value
}

// ParseStringSlice extracts a comma-separated string-slice flag's value from
// raw args, or nil if the flag isn't present. Handles both `--flag=a,b` and
// `--flag a,b` syntax via pflag's own StringSlice parser.
func ParseStringSlice(args []string, flagName string) []string {
	defer perf.Track(nil, "osargs.ParseStringSlice")()

	fs := newFlagSet(flagName + "-parser")
	values := fs.StringSlice(flagName, []string{}, "")
	_ = fs.Parse(args) // Ignore errors from unknown flags.
	if len(*values) == 0 {
		return nil
	}
	return *values
}
