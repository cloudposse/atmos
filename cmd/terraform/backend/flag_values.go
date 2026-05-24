package backend

import (
	"strconv"

	"github.com/spf13/cobra"
)

func getCommandFlagString(cmd *cobra.Command, name string) string {
	if flag := cmd.Flags().Lookup(name); flag != nil && flag.Changed {
		return flag.Value.String()
	}
	if flag := cmd.InheritedFlags().Lookup(name); flag != nil && flag.Changed {
		return flag.Value.String()
	}
	return ""
}

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
