package registry

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/toolchain"
	toolchainregistry "github.com/cloudposse/atmos/toolchain/registry"
)

var (
	searchLimit         int
	searchRegistry      string
	searchFormat        string
	searchInstalledOnly bool
	searchAvailableOnly bool
)

// searchCmd represents the 'toolchain registry search' command.
var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search for tools across registries",
	Long: `Search for tools matching the query string across all configured registries.

The query is matched against tool owner, repo name, and description.
Results are sorted by relevance score.`,
	Args:          cobra.ExactArgs(1),
	RunE:          executeSearchCommand,
	SilenceUsage:  true, // Don't show usage on error.
	SilenceErrors: true, // Don't show errors twice.
}

func init() {
	searchCmd.Flags().IntVar(&searchLimit, "limit", 20, "Maximum number of results to show")
	searchCmd.Flags().StringVar(&searchRegistry, "registry", "", "Search only in specific registry")
	searchCmd.Flags().StringVar(&searchFormat, "format", "table", "Output format (table, json, yaml)")
	searchCmd.Flags().BoolVar(&searchInstalledOnly, "installed-only", false, "Show only installed tools")
	searchCmd.Flags().BoolVar(&searchAvailableOnly, "available-only", false, "Show only non-installed tools")
}

func executeSearchCommand(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "registry.executeSearchCommand")()

	query := args[0]
	ctx := context.Background()

	// Create registry based on flag or use default.
	var reg toolchainregistry.ToolRegistry
	if searchRegistry != "" {
		switch searchRegistry {
		case "aqua-public", "aqua":
			reg = toolchain.NewAquaRegistry()
		default:
			return fmt.Errorf("%w: %s (supported: aqua-public)", toolchainregistry.ErrUnknownRegistry, searchRegistry)
		}
	} else {
		// Use default aqua registry for MVP.
		reg = toolchain.NewAquaRegistry()
	}

	// Search for tools.
	opts := []toolchainregistry.SearchOption{
		toolchainregistry.WithLimit(searchLimit),
	}
	if searchInstalledOnly {
		opts = append(opts, toolchainregistry.WithInstalledOnly(true))
	}
	if searchAvailableOnly {
		opts = append(opts, toolchainregistry.WithAvailableOnly(true))
	}

	results, err := reg.Search(ctx, query, opts...)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	if len(results) == 0 {
		u.PrintfMessageToTUI("No tools found matching '%s'\n", query)
		u.PrintfMessageToTUI("\nTry:\n")
		u.PrintfMessageToTUI("  - Using a different search term\n")
		u.PrintfMessageToTUI("  - Checking 'atmos toolchain registry list' for available tools\n")
		return nil
	}

	// Display results.
	u.PrintfMarkdownToTUI("**Found %d tools matching '%s':**\n\n", len(results), query)
	displaySearchResults(results)

	u.PrintfMessageToTUI("\nUse 'atmos toolchain info <tool>' for details\n")
	u.PrintfMessageToTUI("Use 'atmos toolchain install <tool>@<version>' to install\n")

	return nil
}

func displaySearchResults(tools []*toolchainregistry.Tool) {
	defer perf.Track(nil, "registry.displaySearchResults")()

	// Create table columns.
	columns := []table.Column{
		{Title: "TOOL", Width: 30},
		{Title: "TYPE", Width: 15},
		{Title: "REGISTRY", Width: 20},
	}

	// Convert tools to table rows.
	var rows []table.Row
	for _, tool := range tools {
		toolName := fmt.Sprintf("%s/%s", tool.RepoOwner, tool.RepoName)
		rows = append(rows, table.Row{
			toolName,
			tool.Type,
			tool.Registry,
		})
	}

	// Create and configure table.
	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(false),
		table.WithHeight(len(rows)),
	)

	// Apply theme styles.
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(theme.ColorBorder)).
		BorderBottom(true).
		Bold(true)
	s.Selected = s.Cell

	t.SetStyles(s)

	// Print table.
	fmt.Println(t.View())
}
