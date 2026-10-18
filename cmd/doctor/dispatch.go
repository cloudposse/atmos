package doctor

import (
	"strings"

	"github.com/spf13/pflag"

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

	for len(args) > 0 {
		arg := args[0]
		if arg == flagTerminator {
			return false
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			return arg == commandName
		}
		args = args[1+leadingFlagValues(arg, args[1:], rootFlags):]
	}
	return false
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
		// Optional-value flags consume the next argument only when it is not itself a flag.
		if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
			return 1
		}
		return 0
	case len(rest) > 0:
		return 1
	}
	return 0
}

func isBoolean(flag *pflag.Flag) bool {
	return flag.Value.Type() == "bool"
}

// boolValue stands in for Cobra's lazily registered help flag, which is not a persistent root flag.
type boolValue struct{}

func (boolValue) String() string   { return "false" }
func (boolValue) Set(string) error { return nil }
func (boolValue) Type() string     { return "bool" }
