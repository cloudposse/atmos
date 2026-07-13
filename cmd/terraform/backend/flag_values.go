package backend

import (
	"strconv"

	"github.com/spf13/cobra"
)

// getCommandFlagString reads a flag's value directly off the command, preferring the CLI-
// supplied value (Changed==true) over Viper. This is needed because Viper's precedence gives
// an explicit viper.Set() call priority over a bound pflag, so callers that pre-seed Viper
// (e.g. from config defaults) can otherwise shadow a value the user just passed on the CLI.
func getCommandFlagString(cmd *cobra.Command, name string) string {
	if flag := cmd.Flags().Lookup(name); flag != nil && flag.Changed {
		return flag.Value.String()
	}
	if flag := cmd.InheritedFlags().Lookup(name); flag != nil && flag.Changed {
		return flag.Value.String()
	}
	return ""
}

// getCommandFlagBool is the boolean counterpart to getCommandFlagString. The second return
// value reports whether the flag was explicitly changed on the CLI.
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
