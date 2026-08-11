package flags

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/cloudposse/atmos/pkg/perf"
)

// ResolvedStringFlag reports whether a string flag was explicitly provided and
// returns args with that flag removed from the pre-separator argument stream.
type ResolvedStringFlag struct {
	Value   string
	Changed bool
	Args    []string
}

// ResolveExplicitStringFlag resolves a string flag from Cobra's parsed flags,
// falling back to raw args for commands with DisableFlagParsing.
func ResolveExplicitStringFlag(cmd *cobra.Command, args []string, flagName string) (ResolvedStringFlag, error) {
	defer perf.Track(nil, "flags.ResolveExplicitStringFlag")()

	result := ResolvedStringFlag{Args: args}
	if flag, changed := lookupCommandFlag(cmd, flagName); changed {
		result.Value = flag.Value.String()
		result.Changed = true
		result.Args = stripStringFlagArgs(cmd, args, flagName)
		return result, nil
	}

	if cmd == nil || !cmd.DisableFlagParsing {
		return result, nil
	}

	return resolveExplicitStringFlagFromArgs(cmd, args, flagName), nil
}

// IsHelpRequested reports whether the command invocation is a help request.
func IsHelpRequested(cmd *cobra.Command, args []string) bool {
	defer perf.Track(nil, "flags.IsHelpRequested")()

	if cmd == nil {
		return false
	}
	if cmd.Name() == "help" {
		return true
	}
	if _, changed := lookupCommandFlag(cmd, "help"); changed {
		return true
	}
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == "help" || arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

func resolveExplicitStringFlagFromArgs(cmd *cobra.Command, args []string, flagName string) ResolvedStringFlag {
	result := ResolvedStringFlag{Args: make([]string, 0, len(args))}
	noOptDefVal := stringFlagNoOptDefVal(cmd, flagName)
	consumesNext := stringFlagConsumesNextArg(flagName)

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			result.Args = append(result.Args, args[i:]...)
			break
		}

		if matchesStringFlagAlias(cmd, flagName, arg) {
			i = consumeStringFlagValue(&result, args, i, noOptDefVal, consumesNext)
			continue
		}

		if value, ok := inlineStringFlagValue(cmd, flagName, arg); ok {
			result.Value = value
			result.Changed = true
			continue
		}

		result.Args = append(result.Args, arg)
	}

	return result
}

func consumeStringFlagValue(result *ResolvedStringFlag, args []string, index int, noOptDefVal string, consumesNext bool) int {
	result.Changed = true
	if noOptDefVal != "" && !consumesNext {
		result.Value = noOptDefVal
		return index
	}
	if consumesNext && index+1 < len(args) && !strings.HasPrefix(args[index+1], "-") {
		result.Value = args[index+1]
		return index + 1
	}
	if noOptDefVal != "" {
		result.Value = noOptDefVal
	}
	return index
}

func stripStringFlagArgs(cmd *cobra.Command, args []string, flagName string) []string {
	return resolveExplicitStringFlagFromArgs(cmd, args, flagName).Args
}

func matchesStringFlagAlias(cmd *cobra.Command, flagName, arg string) bool {
	for _, alias := range stringFlagAliases(cmd, flagName) {
		if arg == alias {
			return true
		}
	}
	return false
}

func inlineStringFlagValue(cmd *cobra.Command, flagName, arg string) (string, bool) {
	for _, alias := range stringFlagAliases(cmd, flagName) {
		prefix := alias + "="
		if strings.HasPrefix(arg, prefix) {
			return strings.TrimPrefix(arg, prefix), true
		}
	}
	return "", false
}

func stringFlagAliases(cmd *cobra.Command, flagName string) []string {
	aliases := []string{"--" + flagName}
	if flag := lookupAnyFlag(cmd, flagName); flag != nil && flag.Shorthand != "" {
		aliases = append(aliases, "-"+flag.Shorthand)
	}
	return aliases
}

func stringFlagNoOptDefVal(cmd *cobra.Command, flagName string) string {
	if flag := lookupAnyFlag(cmd, flagName); flag != nil && flag.NoOptDefVal != "" {
		return flag.NoOptDefVal
	}
	if flag := GlobalFlagsRegistry().Get(flagName); flag != nil {
		return flag.GetNoOptDefVal()
	}
	return ""
}

func stringFlagConsumesNextArg(flagName string) bool {
	if flag := GlobalFlagsRegistry().Get(flagName); flag != nil {
		return flag.GetNoOptDefValConsumesNextArg()
	}
	return true
}

func lookupAnyFlag(cmd *cobra.Command, flagName string) *pflag.Flag {
	flag, _ := lookupCommandFlag(cmd, flagName)
	return flag
}
