package registry

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/toolchain"
	toolchainregistry "github.com/cloudposse/atmos/toolchain/registry"
)

const (
	defaultSearchLimit      = 20
	columnWidthTool         = 30
	columnWidthToolType     = 15
	columnWidthToolRegistry = 20
)

var searchParser *flags.StandardParser

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
	// Create parser with search-specific flags.
	searchParser = flags.NewStandardParser(
		flags.WithIntFlag("limit", "", defaultSearchLimit, "Maximum number of results to show"),
		flags.WithStringFlag("registry", "", "", "Search only in specific registry"),
		flags.WithStringFlag("format", "", "table", "Output format (table, json, yaml)"),
		flags.WithBoolFlag("installed-only", "", false, "Show only installed tools"),
		flags.WithBoolFlag("available-only", "", false, "Show only non-installed tools"),
		flags.WithEnvVars("limit", "ATMOS_TOOLCHAIN_LIMIT"),
		flags.WithEnvVars("registry", "ATMOS_TOOLCHAIN_REGISTRY"),
		flags.WithEnvVars("format", "ATMOS_TOOLCHAIN_FORMAT"),
		flags.WithEnvVars("installed-only", "ATMOS_TOOLCHAIN_INSTALLED_ONLY"),
		flags.WithEnvVars("available-only", "ATMOS_TOOLCHAIN_AVAILABLE_ONLY"),
	)

	// Register flags.
	searchParser.RegisterFlags(searchCmd)

	// Bind flags to Viper.
	if err := searchParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func executeSearchCommand(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "registry.executeSearchCommand")()

	// Bind flags to Viper for precedence handling.
	v := viper.GetViper()
	if err := searchParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	query := args[0]
	ctx := context.Background()

	// Get flag values from Viper.
	searchLimit := v.GetInt("limit")
	searchRegistry := v.GetString("registry")
	searchInstalledOnly := v.GetBool("installed-only")
	searchAvailableOnly := v.GetBool("available-only")
	searchFormat := strings.ToLower(v.GetString("format"))

	// Validate format flag.
	switch searchFormat {
	case "table", "json", "yaml":
		// Valid formats.
	default:
		return fmt.Errorf("%w: format must be one of: table, json, yaml (got: %s)",
			errUtils.ErrInvalidFlag, searchFormat)
	}

	// Create registry based on flag or use default.
	var reg toolchainregistry.ToolRegistry
	if searchRegistry != "" {
		switch searchRegistry {
		case "aqua-public", "aqua":
			reg = toolchain.NewAquaRegistry()
		default:
			return fmt.Errorf("%w: '%s' (supported registries: 'aqua-public', 'aqua')", toolchainregistry.ErrUnknownRegistry, searchRegistry)
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
		message := fmt.Sprintf(`No tools found matching '%s'

Try:
  - Using a different search term
  - Checking 'atmos toolchain registry list' for available tools
`, query)
		_ = ui.Info(message)
		return nil
	}

	// Output based on format.
	switch searchFormat {
	case "json":
		return data.WriteJSON(results)
	case "yaml":
		return data.WriteYAML(results)
	case "table":
		// Display results.
		_ = ui.MarkdownMessagef("**Found %d tools matching '%s':**\n\n", len(results), query)
		displaySearchResults(results)

		footer := `
Use 'atmos toolchain info <tool>' for details
Use 'atmos toolchain install <tool>@<version>' to install
`
		_ = ui.Write(footer)
		return nil
	default:
		// Should never reach here due to validation above.
		return fmt.Errorf("%w: unsupported format: %s", errUtils.ErrInvalidFlag, searchFormat)
	}
}

func displaySearchResults(tools []*toolchainregistry.Tool) {
	defer perf.Track(nil, "registry.displaySearchResults")()

	// Create table columns.
	columns := []table.Column{
		{Title: "TOOL", Width: columnWidthTool},
		{Title: "TYPE", Width: columnWidthToolType},
		{Title: "REGISTRY", Width: columnWidthToolRegistry},
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

// SearchCommandProvider implements the CommandProvider interface.
type SearchCommandProvider struct{}

func (s *SearchCommandProvider) GetCommand() *cobra.Command {
	return searchCmd
}

func (s *SearchCommandProvider) GetName() string {
	return "search"
}

func (s *SearchCommandProvider) GetGroup() string {
	return "Toolchain Commands"
}

func (s *SearchCommandProvider) GetFlagsBuilder() flags.Builder {
	return searchParser
}

func (s *SearchCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (s *SearchCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// GetSearchParser returns the search command's parser for use by aliases.
func GetSearchParser() *flags.StandardParser {
	return searchParser
}
