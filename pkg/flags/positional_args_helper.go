package flags

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	singleDashArg           = "-"
	flagPrefix              = "-"
	longFlagPrefix          = "--"
	endOfOptionsArg         = "--"
	flagAssignmentSeparator = "="
)

// FirstPositionalArg returns the first positional argument in args without
// parsing flags or mutating command state. It uses Cobra flag metadata only to
// skip values belonging to flags before the first positional argument.
func FirstPositionalArg(cmd *cobra.Command, args []string) string {
	defer perf.Track(nil, "flags.FirstPositionalArg")()

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == endOfOptionsArg {
			return ""
		}
		if !strings.HasPrefix(arg, flagPrefix) {
			return arg
		}
		if flagConsumesNextValue(cmd, arg) && i+1 < len(args) && args[i+1] != endOfOptionsArg && !strings.HasPrefix(args[i+1], flagPrefix) {
			i++
		}
	}
	return ""
}

func flagConsumesNextValue(cmd *cobra.Command, arg string) bool {
	if arg == singleDashArg || arg == endOfOptionsArg || strings.Contains(arg, flagAssignmentSeparator) {
		return false
	}
	if hasAttachedShorthandValue(cmd, arg) {
		return false
	}

	return flagRequiresNextValue(lookupFlagForArg(cmd, arg))
}

func hasAttachedShorthandValue(cmd *cobra.Command, arg string) bool {
	if cmd == nil || !strings.HasPrefix(arg, flagPrefix) || strings.HasPrefix(arg, longFlagPrefix) {
		return false
	}
	name := strings.TrimPrefix(arg, flagPrefix)
	if len(name) <= 1 {
		return false
	}
	return flagConsumesAttachedValue(cmd.Flags().ShorthandLookup(name[:1]))
}

func flagRequiresNextValue(flag *pflag.Flag) bool {
	if flag == nil {
		// Unknown flags are treated conservatively: skip the following value
		// rather than risk exposing a flag value in labels or other summaries.
		return true
	}
	if flag.NoOptDefVal != "" {
		return false
	}
	if flag.Value != nil && flag.Value.Type() == "bool" {
		return false
	}
	return true
}

func lookupFlagForArg(cmd *cobra.Command, arg string) *pflag.Flag {
	if cmd == nil {
		return nil
	}

	if strings.HasPrefix(arg, longFlagPrefix) {
		name := strings.TrimPrefix(arg, longFlagPrefix)
		if idx := strings.Index(name, flagAssignmentSeparator); idx >= 0 {
			name = name[:idx]
		}
		return cmd.Flags().Lookup(name)
	}

	name := strings.TrimPrefix(arg, flagPrefix)
	if idx := strings.Index(name, flagAssignmentSeparator); idx >= 0 {
		name = name[:idx]
	}
	if len(name) == 1 {
		return cmd.Flags().ShorthandLookup(name)
	}
	if flag := cmd.Flags().Lookup(name); flag != nil {
		return flag
	}
	if len(name) > 0 {
		return cmd.Flags().ShorthandLookup(name[:1])
	}
	return nil
}

func flagConsumesAttachedValue(flag *pflag.Flag) bool {
	if flag == nil {
		return false
	}
	if flag.NoOptDefVal != "" {
		return false
	}
	if flag.Value != nil && flag.Value.Type() == "bool" {
		return false
	}
	return true
}
