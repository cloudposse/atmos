package internal

import (
	"maps"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

// ScriptCommandCatalog exposes the registered CLI to embedded script bindings
// without sharing mutable command or flag objects with script goroutines.
func ScriptCommandCatalog(root *cobra.Command) *flags.CommandCatalog {
	return flags.NewCommandCatalog(root, func(path []string) map[string]compat.CompatibilityFlag {
		result := maps.Clone(GetCompatFlagsForCommand(path[0]))
		if len(path) > 1 {
			if result == nil {
				result = make(map[string]compat.CompatibilityFlag)
			}
			maps.Copy(result, GetSubcommandCompatFlags(path[0], strings.Join(path[1:], "-")))
		}
		return result
	})
}
