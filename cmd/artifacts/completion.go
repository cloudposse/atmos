package artifacts

import (
	"sort"

	"github.com/spf13/cobra"
)

// sortStrings sorts in place; a named helper keeps call sites readable.
func sortStrings(values []string) { sort.Strings(values) }

// registerCompletions offers the output formats and the configured repository aliases. Completion
// reads configuration only; it never authenticates.
func registerCompletions(cmd *cobra.Command, formats []string) {
	if cmd.Flags().Lookup(flagFormat) != nil {
		_ = cmd.RegisterFlagCompletionFunc(flagFormat, func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return formats, cobra.ShellCompDirectiveNoFileComp
		})
	}
	for _, name := range []string{flagRepository, flagFrom, flagTo} {
		if cmd.Flags().Lookup(name) == nil {
			continue
		}
		_ = cmd.RegisterFlagCompletionFunc(name, func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return configuredAliases(atmosConfigPtr), cobra.ShellCompDirectiveNoFileComp
		})
	}
}
