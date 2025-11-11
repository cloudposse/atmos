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
		// Display results with info toast.
		_ = ui.Infof("Found %d tools matching '%s':\n", len(results), query)
		displaySearchResults(results)

		// Show helpful hints after table.
		_ = ui.Writeln("")
		_ = ui.Hintf("Use `atmos toolchain info <tool>` for details")
		_ = ui.Hintf("Use `atmos toolchain install <tool>@<version>` to install")
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

	// Load tool versions to check installation status.
	installer := toolchain.NewInstaller()
	toolVersionsFile := toolchain.GetToolVersionsFilePath()
	toolVersions, err := toolchain.LoadToolVersions(toolVersionsFile)
	if err != nil && !os.IsNotExist(err) {
		// If there's an error other than file not found, log it but continue.
		ui.Warningf("Could not load .tool-versions: %v", err)
	}

	// Build row data with installation status.
	var rows []searchRow
	statusWidth := 2 // For dot character.
	toolNameWidth := len("TOOL")
	typeWidth := len("TYPE")
	registryWidth := len("REGISTRY")

	// Check each tool's installation and configuration status.
	for _, tool := range tools {
		toolName := fmt.Sprintf("%s/%s", tool.RepoOwner, tool.RepoName)
		row := searchRow{
			toolName: toolName,
			toolType: tool.Type,
			registry: tool.Registry,
		}

		// Check if tool is in configuration.
		if toolVersions != nil && toolVersions.Tools != nil {
			// Check both full name and repo name as alias.
			fullName := tool.RepoOwner + "/" + tool.RepoName
			_, foundFull := toolVersions.Tools[fullName]
			_, foundRepo := toolVersions.Tools[tool.RepoName]
			row.isInConfig = foundFull || foundRepo

			// Check if installed (only if in config).
			if row.isInConfig {
				// Get the version from tool-versions.
				var version string
				if foundFull {
					versions := toolVersions.Tools[fullName]
					if len(versions) > 0 {
						version = versions[0]
					}
				} else if foundRepo {
					versions := toolVersions.Tools[tool.RepoName]
					if len(versions) > 0 {
						version = versions[0]
					}
				}

				// Check if binary exists.
				if version != "" {
					_, err := installer.FindBinaryPath(tool.RepoOwner, tool.RepoName, version, tool.BinaryName)
					row.isInstalled = err == nil
				}
			}
		}

		// Set status indicator.
		if row.isInstalled {
			row.status = "●" // Green dot (will be colored later).
		} else if row.isInConfig {
			row.status = "●" // Gray dot (will be colored later).
		} else {
			row.status = " " // No indicator.
		}

		// Update column widths.
		if len(toolName) > toolNameWidth {
			toolNameWidth = len(toolName)
		}
		if len(tool.Type) > typeWidth {
			typeWidth = len(tool.Type)
		}
		if len(tool.Registry) > registryWidth {
			registryWidth = len(tool.Registry)
		}

		rows = append(rows, row)
	}

	// Add padding.
	const columnPaddingPerSide = 2
	const totalColumnPadding = columnPaddingPerSide * 2
	statusWidth += totalColumnPadding
	toolNameWidth += totalColumnPadding
	typeWidth += totalColumnPadding
	registryWidth += totalColumnPadding

	// Create table columns with calculated widths.
	columns := []table.Column{
		{Title: " ", Width: statusWidth}, // Status column.
		{Title: "TOOL", Width: toolNameWidth},
		{Title: "TYPE", Width: typeWidth},
		{Title: "REGISTRY", Width: registryWidth},
	}

	// Convert rows to table format.
	var tableRows []table.Row
	for _, row := range rows {
		tableRows = append(tableRows, table.Row{
			row.status,
			row.toolName,
			row.toolType,
			row.registry,
		})
	}

	// Create and configure table.
	t := table.New(
		table.WithColumns(columns),
		table.WithRows(tableRows),
		table.WithFocused(false),
		table.WithHeight(len(tableRows)),
	)

	// Apply theme styles.
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(theme.ColorBorder)).
		BorderBottom(true).
		Bold(true)
	s.Cell = s.Cell.PaddingLeft(1).PaddingRight(1)
	s.Selected = s.Cell

	t.SetStyles(s)

	// Render table and apply conditional styling.
	styled := renderSearchTableWithConditionalStyling(t.View(), rows)
	_ = data.Writeln(styled)
}

// renderSearchTableWithConditionalStyling applies color to status indicators and dims non-installed rows.
func renderSearchTableWithConditionalStyling(tableView string, rows []searchRow) string {
	lines := strings.Split(tableView, "\n")

	// Define styles.
	greenDot := lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // Green for installed.
	grayDot := lipgloss.NewStyle().Foreground(lipgloss.Color("240")) // Gray for in config but not installed.
	grayRow := lipgloss.NewStyle().Foreground(lipgloss.Color("240")) // Gray for entire uninstalled row.

	// Apply conditional styling to each row.
	for i, line := range lines {
		if i == 0 || i == 1 {
			// Header and border lines - keep as is.
			continue
		}

		// Skip empty lines.
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Map line to row data (adjust index for header and border).
		rowIndex := i - 2
		if rowIndex >= 0 && rowIndex < len(rows) {
			rowData := rows[rowIndex]

			// Color the status dot and apply row styling.
			if rowData.isInstalled {
				// Replace the dot with a green dot.
				line = strings.Replace(line, "●", greenDot.Render("●"), 1)
			} else if rowData.isInConfig {
				// Replace the dot with a gray dot and gray the entire row.
				line = strings.Replace(line, "●", grayDot.Render("●"), 1)
				line = grayRow.Render(line)
			}

			lines[i] = line
		}
	}

	return strings.Join(lines, "\n")
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
