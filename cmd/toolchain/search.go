package toolchain

import (
	"github.com/spf13/cobra"

	registrycmd "github.com/cloudposse/atmos/cmd/toolchain/registry"
)

// searchAliasCmd is an alias to 'toolchain registry search'.
var searchAliasCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search for tools (alias to 'registry search')",
	Long: `Search for tools matching the query string across all configured registries.

This is an alias to 'atmos toolchain registry search' for convenience.

The query is matched against tool owner, repo name, and description.
Results are sorted by relevance score.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Delegate directly to registry search command implementation.
		registryCmd := registrycmd.GetRegistryCommand()
		searchCmd, _, err := registryCmd.Find([]string{"search"})
		if err != nil {
			return err
		}

		// Call the RunE function directly instead of Execute() to avoid recursion.
		return searchCmd.RunE(searchCmd, args)
	},
}
