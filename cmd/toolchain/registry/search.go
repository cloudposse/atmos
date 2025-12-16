package registry

import (
	"context"
	"fmt"
	"os"
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

// validateSearchFormat validates the search format flag.
func validateSearchFormat(format string) error {
	switch format {
	case "table", "json", "yaml":
		return nil
	default:
		return fmt.Errorf("%w: format must be one of: table, json, yaml (got: %s)",
			errUtils.ErrInvalidFlag, format)
	}
}

// createSearchRegistry creates a registry based on the name.
func createSearchRegistry(registryName string) (toolchainregistry.ToolRegistry, error) {
	if registryName == "" {
		// Use default aqua registry for MVP.
		return toolchain.NewAquaRegistry(), nil
	}

	switch registryName {
	case "aqua-public", "aqua":
		return toolchain.NewAquaRegistry(), nil
	default:
		return nil, fmt.Errorf("%w: '%s' (supported registries: 'aqua-public', 'aqua')",
			toolchainregistry.ErrUnknownRegistry, registryName)
	}
}

// displaySearchTable displays search results in table format.
func displaySearchTable(results []*toolchainregistry.Tool, query string, searchLimit int) {
	totalMatches := len(results)
	displayResults := results
	// Only apply limit when searchLimit > 0 (0 means no limit).
	if searchLimit > 0 && totalMatches > searchLimit {
		displayResults = results[:searchLimit]
	}

	// Display results with info toast showing range vs total.
	if searchLimit <= 0 || totalMatches <= searchLimit {
		// Showing all results (no limit or within limit).
		_ = ui.Infof("Found **%d tools** matching `%s`:", totalMatches, query)
	} else {
		// Showing subset of results.
		_ = ui.Infof("Showing **%d** of **%d tools** matching `%s`:", len(displayResults), totalMatches, query)
	}
	_ = ui.Writeln("") // Blank line after toast.
	displaySearchResults(displayResults)

	// Show helpful hints after table.
	_ = ui.Writeln("")
	_ = ui.Hintf("Use `atmos toolchain info <tool>` for details")
	_ = ui.Hintf("Use `atmos toolchain install <tool>@<version>` to install")
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
	if err := validateSearchFormat(searchFormat); err != nil {
		return err
	}

	// Validate limit flag.
	if searchLimit < 0 {
		return fmt.Errorf("%w: limit must be non-negative", errUtils.ErrInvalidFlag)
	}

	// Validate conflicting filter flags.
	if searchInstalledOnly && searchAvailableOnly {
		return fmt.Errorf("%w: cannot use both --installed-only and --available-only", errUtils.ErrInvalidFlag)
	}

	// Create registry based on flag or use default.
	reg, err := createSearchRegistry(searchRegistry)
	if err != nil {
		return err
	}

	// Search for all matching tools (no limit on search itself).
	opts := []toolchainregistry.SearchOption{
		toolchainregistry.WithLimit(0), // 0 = no limit, get all matches
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

	// Apply display limit for JSON/YAML (0 means no limit).
	displayResults := results
	if searchLimit > 0 && len(results) > searchLimit {
		displayResults = results[:searchLimit]
	}

	// Output based on format.
	switch searchFormat {
	case "json":
		return data.WriteJSON(displayResults)
	case "yaml":
		return data.WriteYAML(displayResults)
	case "table":
		displaySearchTable(results, query, searchLimit)
		return nil
	default:
		// Should never reach here due to validation above.
		return fmt.Errorf("%w: unsupported format: %s", errUtils.ErrInvalidFlag, searchFormat)
	}
}

// searchRow represents a single row in the search results table.
type searchRow struct {
	status      string
	toolName    string
	toolType    string
	registry    string
	isInstalled bool
	isInConfig  bool
}

func displaySearchResults(tools []*toolchainregistry.Tool) {
	defer perf.Track(nil, "registry.displaySearchResults")()

	toolVersions, installer := loadSearchToolVersions()
	rows, widths := buildSearchRows(tools, toolVersions, installer)
	styled := renderSearchTable(rows, widths)
	_ = ui.Writeln(styled)
}

// loadSearchToolVersions loads tool versions and creates an installer for search.
func loadSearchToolVersions() (*toolchain.ToolVersions, *toolchain.Installer) {
	installer := toolchain.NewInstaller()
	toolVersionsFile := toolchain.GetToolVersionsFilePath()
	toolVersions, err := toolchain.LoadToolVersions(toolVersionsFile)
	if err != nil && !os.IsNotExist(err) {
		_ = ui.Warningf("Could not load .tool-versions: %v", err)
	}
	return toolVersions, installer
}

// searchColumnWidths holds the calculated widths for search table columns.
type searchColumnWidths struct {
	status   int
	toolName int
	toolType int
	registry int
}

// buildSearchRows processes tools and returns search rows with column widths.
func buildSearchRows(tools []*toolchainregistry.Tool, toolVersions *toolchain.ToolVersions, installer *toolchain.Installer) ([]searchRow, searchColumnWidths) {
	widths := searchColumnWidths{
		status:   1,
		toolName: len("TOOL"),
		toolType: len("TYPE"),
		registry: len("REGISTRY"),
	}

	rows := make([]searchRow, 0, len(tools))
	for _, tool := range tools {
		row := buildSingleSearchRow(tool, toolVersions, installer)
		widths = updateSearchColumnWidths(widths, tool, row.toolName)
		rows = append(rows, row)
	}
	return rows, widths
}

// buildSingleSearchRow creates a searchRow for a single tool.
func buildSingleSearchRow(tool *toolchainregistry.Tool, toolVersions *toolchain.ToolVersions, installer *toolchain.Installer) searchRow {
	toolName := fmt.Sprintf("%s/%s", tool.RepoOwner, tool.RepoName)
	row := searchRow{
		toolName: toolName,
		toolType: tool.Type,
		registry: tool.Registry,
	}

	if toolVersions != nil && toolVersions.Tools != nil {
		row.isInConfig, row.isInstalled = checkSearchToolStatus(tool, toolVersions, installer)
	}

	row.status = getSearchStatusIndicator(row.isInstalled, row.isInConfig)
	return row
}

// checkSearchToolStatus checks if a tool is in config and installed.
func checkSearchToolStatus(tool *toolchainregistry.Tool, toolVersions *toolchain.ToolVersions, installer *toolchain.Installer) (inConfig, installed bool) {
	fullName := tool.RepoOwner + "/" + tool.RepoName
	_, foundFull := toolVersions.Tools[fullName]
	_, foundRepo := toolVersions.Tools[tool.RepoName]
	inConfig = foundFull || foundRepo

	if !inConfig {
		return inConfig, false
	}

	version := getSearchToolVersion(fullName, tool.RepoName, toolVersions, foundFull, foundRepo)
	if version == "" {
		return inConfig, false
	}

	_, err := installer.FindBinaryPath(tool.RepoOwner, tool.RepoName, version, tool.BinaryName)
	return inConfig, err == nil
}

// getSearchToolVersion gets the version string for a tool from tool-versions.
func getSearchToolVersion(fullName, repoName string, toolVersions *toolchain.ToolVersions, foundFull, foundRepo bool) string {
	if foundFull {
		if versions := toolVersions.Tools[fullName]; len(versions) > 0 {
			return versions[0]
		}
	} else if foundRepo {
		if versions := toolVersions.Tools[repoName]; len(versions) > 0 {
			return versions[0]
		}
	}
	return ""
}

// getSearchStatusIndicator returns the status indicator character based on tool state.
func getSearchStatusIndicator(isInstalled, isInConfig bool) string {
	switch {
	case isInstalled:
		return statusIndicator
	case isInConfig:
		return statusIndicator
	default:
		return " "
	}
}

// updateSearchColumnWidths updates widths based on a tool's field lengths.
func updateSearchColumnWidths(widths searchColumnWidths, tool *toolchainregistry.Tool, toolName string) searchColumnWidths {
	if len(toolName) > widths.toolName {
		widths.toolName = len(toolName)
	}
	if len(tool.Type) > widths.toolType {
		widths.toolType = len(tool.Type)
	}
	if len(tool.Registry) > widths.registry {
		widths.registry = len(tool.Registry)
	}
	return widths
}

// renderSearchTable creates and renders the search results table with styling.
func renderSearchTable(rows []searchRow, widths searchColumnWidths) string {
	const columnPaddingPerSide = 2
	const totalColumnPadding = columnPaddingPerSide * 2
	const statusPadding = 2
	widths.status += statusPadding
	widths.toolName += totalColumnPadding
	widths.toolType += totalColumnPadding
	widths.registry += totalColumnPadding

	columns := []table.Column{
		{Title: " ", Width: widths.status},
		{Title: "TOOL", Width: widths.toolName},
		{Title: "TYPE", Width: widths.toolType},
		{Title: "REGISTRY", Width: widths.registry},
	}

	tableRows := make([]table.Row, 0, len(rows))
	for _, row := range rows {
		tableRows = append(tableRows, table.Row{row.status, row.toolName, row.toolType, row.registry})
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(tableRows),
		table.WithFocused(false),
		table.WithHeight(len(tableRows)+1),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(theme.ColorBorder)).
		BorderBottom(true).
		Bold(true)
	s.Cell = s.Cell.PaddingLeft(1).PaddingRight(1)
	s.Selected = s.Cell
	t.SetStyles(s)

	return renderTableWithConditionalStyling(t.View(), rows)
}

// SearchCommandProvider implements the CommandProvider interface for the 'toolchain registry search' command, wiring the search subcommand into the CLI framework with its associated flags and behaviors.
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

// GetSearchParser returns the search command's parser for use by aliases and other commands that need access to search-specific flag definitions.
func GetSearchParser() *flags.StandardParser {
	return searchParser
}
