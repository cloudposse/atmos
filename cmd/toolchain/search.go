package toolchain

import (
	"github.com/spf13/cobra"

	registrycmd "github.com/cloudposse/atmos/cmd/toolchain/registry"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

// searchAliasCmd is an alias to 'toolchain registry search'.
var searchAliasCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search for tools (alias to 'registry search')",
	Long: `Search for tools matching the query string across all configured registries.

This is an alias to 'atmos toolchain registry search' for convenience.

The query is matched against tool owner, repo name, and description.
Results are sorted by relevance score.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get the actual search command from registry parent.
		registryCmd := registrycmd.GetRegistryCommand()
		searchCmd, _, err := registryCmd.Find([]string{"search"})
		if err != nil {
			return err
		}

		// Forward args and execute through Cobra to ensure proper flag parsing and PreRun execution.
		searchCmd.SetArgs(args)
		return searchCmd.ExecuteContext(cmd.Context())
	},
}

func init() {
	// Register flags from the actual search command on the alias.
	// This ensures flags work on the alias too.
	searchParser := registrycmd.GetSearchParser()
	if searchParser != nil {
		searchParser.RegisterFlags(searchAliasCmd)
	}
}

// SearchCommandProvider implements the CommandProvider interface.
type SearchCommandProvider struct{}

func (s *SearchCommandProvider) GetCommand() *cobra.Command {
	return searchAliasCmd
}

func (s *SearchCommandProvider) GetName() string {
	return "search"
}

func (s *SearchCommandProvider) GetGroup() string {
	return "Toolchain Commands"
}

func (s *SearchCommandProvider) GetFlagsBuilder() flags.Builder {
	return registrycmd.GetSearchParser()
}

func (s *SearchCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (s *SearchCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}
