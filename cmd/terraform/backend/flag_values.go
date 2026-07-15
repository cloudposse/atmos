package backend

import (
	"strconv"

	"github.com/spf13/cobra"
)

// getCommandFlagStack is used only as a compatibility fallback for tests and callers that
// invoke RunE directly without Cobra first parsing the command's flag set. Normal execution
// obtains parsed values from StandardParser. Identity never uses this path.
func getCommandFlagStack(cmd *cobra.Command) string {
	if flag := cmd.Flags().Lookup("stack"); flag != nil && flag.Changed {
		return flag.Value.String()
	}
	if flag := cmd.InheritedFlags().Lookup("stack"); flag != nil && flag.Changed {
		return flag.Value.String()
	}
	return ""
}

// getCommandFlagBool reads the force flag, whose value is not part of StandardOptions. The
// second return value reports whether the flag was explicitly changed on the CLI.
func getCommandFlagBool(cmd *cobra.Command, name string) (bool, bool) {
	if flag := cmd.Flags().Lookup(name); flag != nil && flag.Changed {
		value, err := strconv.ParseBool(flag.Value.String())
		if err == nil {
			return value, true
		}
	}
	if flag := cmd.InheritedFlags().Lookup(name); flag != nil && flag.Changed {
		value, err := strconv.ParseBool(flag.Value.String())
		if err == nil {
			return value, true
		}
	}
	return false, false
}
