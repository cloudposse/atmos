package doctor

import (
	"strings"
	"sync"

	"github.com/spf13/pflag"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	terminalenv "github.com/cloudposse/atmos/pkg/terminal/env"
)

const (
	flagTerminator = "--"
	equalsSign     = "="
	helpFlagName   = "help"
	helpShorthand  = "h"
	commandName    = "doctor"
)

// IsInvocation reports whether args (the arguments after the program name) invoke
// the doctor command once leading root-level flags are skipped.
//
// Flag arity is derived from the root command's persistent flags rather than a
// hand-maintained allowlist, so a newly added root flag cannot silently route
// `atmos <flag> doctor` through normal configuration loading, which would crash
// on exactly the malformed configuration that doctor exists to diagnose.
// Unknown leading flags are treated as value-less: doctor then reaches Cobra, which
// reports the unknown flag, instead of the configuration loader.
func IsInvocation(args []string, rootFlags *pflag.FlagSet) bool {
	defer perf.Track(nil, "doctor.IsInvocation")()

	_, ok := InvocationIndex(args, rootFlags)
	return ok
}

// InvocationIndex reports whether args invoke the doctor command and, when they do, the index of the
// "doctor" token. Callers use the index to treat the leading root flags and the command's own arguments
// separately, because an optional-value root flag such as --pager must never absorb the command name.
func InvocationIndex(args []string, rootFlags *pflag.FlagSet) (int, bool) {
	defer perf.Track(nil, "doctor.InvocationIndex")()

	for i := 0; i < len(args); {
		arg := args[i]
		if arg == flagTerminator {
			return 0, false
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			return i, arg == commandName
		}
		i += 1 + leadingFlagValues(arg, args[i+1:], rootFlags)
	}
	return 0, false
}

// leadingFlagValues returns how many arguments following flag arg it consumes as its value.
func leadingFlagValues(arg string, rest []string, rootFlags *pflag.FlagSet) int {
	if strings.HasPrefix(arg, "--") {
		name, inline := splitLongFlag(arg)
		if inline {
			return 0
		}
		return valuesForFlag(lookupLong(rootFlags, name), rest)
	}
	return shorthandValues(arg[1:], rest, rootFlags)
}

// splitLongFlag splits "--name=value" into the flag name and whether a value was supplied inline.
func splitLongFlag(arg string) (name string, inline bool) {
	name, _, inline = strings.Cut(strings.TrimPrefix(arg, "--"), equalsSign)
	return name, inline
}

func lookupLong(rootFlags *pflag.FlagSet, name string) *pflag.Flag {
	if rootFlags == nil {
		return nil
	}
	return rootFlags.Lookup(name)
}

// shorthandValues handles "-v", "-Cdir", and clustered shorthands such as "-vh".
func shorthandValues(cluster string, rest []string, rootFlags *pflag.FlagSet) int {
	for i := 0; i < len(cluster); i++ {
		flag := lookupShorthand(rootFlags, cluster[i:i+1])
		if flag == nil || isBoolean(flag) {
			continue
		}
		// A value-taking shorthand owns the remainder of the cluster, and "-C=dir" supplies it inline.
		if i+1 < len(cluster) {
			return 0
		}
		return valuesForFlag(flag, rest)
	}
	return 0
}

func lookupShorthand(rootFlags *pflag.FlagSet, shorthand string) *pflag.Flag {
	if shorthand == helpShorthand {
		return &pflag.Flag{Name: helpFlagName, Value: boolValue{}}
	}
	if rootFlags == nil {
		return nil
	}
	return rootFlags.ShorthandLookup(shorthand)
}

// valuesForFlag returns 1 when a bare flag consumes the next argument as its value, else 0.
func valuesForFlag(flag *pflag.Flag, rest []string) int {
	switch {
	case flag == nil:
		// Unknown flags are treated as value-less; Cobra rejects them after dispatch.
		return 0
	case isBoolean(flag):
		// Atmos accepts an explicit "true"/"false" after a bare boolean flag.
		if len(rest) > 0 && terminalenv.IsBoolLiteral(rest[0]) {
			return 1
		}
		return 0
	case flag.NoOptDefVal != "":
		return optionalValueFlagValues(flag, rest)
	case len(rest) > 0:
		return 1
	}
	return 0
}

// optionalValueFlagValues returns how many arguments an optional-value (NoOptDefVal) flag consumes.
// Such a flag consumes the next argument only when it is not itself a flag and is not the doctor command
// name, so "atmos --pager doctor" dispatches doctor. Flags whose registry entry opts out of space-separated
// values (for example --cast) never consume a following token.
func optionalValueFlagValues(flag *pflag.Flag, rest []string) int {
	if registered := globalFlags().Get(flag.Name); registered != nil && !registered.GetNoOptDefValConsumesNextArg() {
		return 0
	}
	if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") && rest[0] != commandName {
		return 1
	}
	return 0
}

var (
	globalFlagsOnce     sync.Once
	globalFlagsRegistry *flags.FlagRegistry
)

// globalFlags returns the registry of global flags, built once on first use.
func globalFlags() *flags.FlagRegistry {
	globalFlagsOnce.Do(func() { globalFlagsRegistry = flags.GlobalFlagsRegistry() })
	return globalFlagsRegistry
}

func isBoolean(flag *pflag.Flag) bool {
	return flag.Value.Type() == "bool"
}

// boolValue stands in for Cobra's lazily registered help flag, which is not a persistent root flag.
type boolValue struct{}

func (boolValue) String() string   { return "false" }
func (boolValue) Set(string) error { return nil }
func (boolValue) Type() string     { return "bool" }
