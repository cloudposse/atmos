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
	"golang.org/x/term"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/toolchain"
	toolchainregistry "github.com/cloudposse/atmos/toolchain/registry"
)

const (
	defaultListLimit     = 50
	minColumnWidthOwner  = 8   // Minimum width for OWNER column.
	minColumnWidthRepo   = 8   // Minimum width for REPO column.
	minColumnWidthType   = 8   // Minimum width for TYPE column.
	defaultTerminalWidth = 120 // Fallback if terminal width cannot be detected.
	columnPaddingPerSide = 2   // Padding on each side of column content.
	totalColumnPadding   = columnPaddingPerSide * 2
	statusIndicator      = "●" // Dot character for installation status
)

// toolRow represents a single row in the tools table.
type toolRow struct {
	status      string
	owner       string
	repo        string
	toolType    string
	isInstalled bool
	isInConfig  bool
}

var listParser *flags.StandardParser

// ListOptions contains parsed flags for the registry list command, including embedded global flags such as pager settings and display options (limit, offset, format, sort).
type ListOptions struct {
	global.Flags // Embed global flags (includes Pager).
	Limit        int
	Offset       int
	Format       string
	Sort         string
}

// listCmd represents the 'toolchain registry list' command.
var listCmd = &cobra.Command{
	Use:   "list [registry-name]",
	Short: "List registries or tools in a registry",
	Long: `List configured registries, or list all tools from a specific registry.

If no registry name is provided, displays all configured registries.
If a registry name is provided, displays all tools in that registry.`,
	Args: cobra.MaximumNArgs(1),
	RunE: executeListCommand,
}

func init() {
	// Create parser with list-specific flags.
	listParser = flags.NewStandardParser(
		flags.WithIntFlag("limit", "", defaultListLimit, "Maximum number of results to show"),
		flags.WithIntFlag("offset", "", 0, "Skip first N results (pagination)"),
		flags.WithStringFlag("format", "", "table", "Output format (table, json, yaml)"),
		flags.WithStringFlag("sort", "", "name", "Sort order (name, date, popularity)"),
		flags.WithEnvVars("limit", "ATMOS_TOOLCHAIN_LIMIT"),
		flags.WithEnvVars("offset", "ATMOS_TOOLCHAIN_OFFSET"),
		flags.WithEnvVars("format", "ATMOS_TOOLCHAIN_FORMAT"),
		flags.WithEnvVars("sort", "ATMOS_TOOLCHAIN_SORT"),
	)

	// Register flags.
	listParser.RegisterFlags(listCmd)

	// Bind flags to Viper.
	if err := listParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func executeListCommand(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "registry.executeListCommand")()

	// Bind flags to Viper for precedence handling.
	v := viper.GetViper()
	if err := listParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	ctx := context.Background()

	// If no registry name provided, list all configured registries.
	if len(args) == 0 {
		return listConfiguredRegistries(ctx)
	}

	// Parse options.
	opts, err := parseListOptions(cmd, v, args)
	if err != nil {
		return err
	}

	// List tools from specific registry.
	registryName := args[0]
	return listRegistryTools(ctx, registryName, opts)
}

func parseListOptions(cmd *cobra.Command, v *viper.Viper, _ []string) (*ListOptions, error) {
	defer perf.Track(nil, "registry.parseListOptions")()

	format := strings.ToLower(v.GetString("format"))

	// Validate format flag.
	switch format {
	case "table", "json", "yaml":
		// Valid formats.
	default:
		return nil, fmt.Errorf("%w: format must be one of: table, json, yaml (got: %s)",
			errUtils.ErrInvalidFlag, format)
	}

	return &ListOptions{
		Flags:  flags.ParseGlobalFlags(cmd, v),
		Limit:  v.GetInt("limit"),
		Offset: v.GetInt("offset"),
		Format: format,
		Sort:   v.GetString("sort"),
	}, nil
}

func listConfiguredRegistries(_ context.Context) error {
	defer perf.Track(nil, "registry.listConfiguredRegistries")()

	// For MVP, show placeholder message.
	// TODO: Load registries from atmos.yaml configuration.
	message := `**Available registries:**

Default built-in registries:
  - aqua-public (aqua, priority: 10)

Use 'atmos toolchain registry list aqua' to see tools in the Aqua registry.
`
	_ = ui.MarkdownMessage(message)

	return nil
}

// displayToolsTable displays tools in table format with paging support.
func displayToolsTable(ctx context.Context, reg toolchainregistry.ToolRegistry, tools []*toolchainregistry.Tool, registryName string, opts *ListOptions, pagerEnabled bool) error {
	// Get metadata for total count.
	meta, err := reg.GetMetadata(ctx)
	if err != nil {
		return fmt.Errorf("failed to get registry metadata: %w", err)
	}

	// Calculate range being displayed.
	start := opts.Offset + 1
	end := opts.Offset + len(tools)
	total := meta.ToolCount

	// Show info toast before pager content.
	if end == total {
		// Showing all tools.
		_ = ui.Infof("Showing **%d tools** from registry `%s` (%s)", total, registryName, meta.Type)
	} else {
		// Showing a subset.
		_ = ui.Infof("Showing **%d-%d** of **%d tools** from registry `%s` (%s)", start, end, total, registryName, meta.Type)
	}
	_ = ui.Writef("Source: %s\n\n", meta.Source)

	// Get table content.
	tableContent := buildToolsTable(tools)

	// Use pager if enabled, otherwise print directly.
	pageCreator := pager.NewWithAtmosConfig(pagerEnabled)
	title := fmt.Sprintf("Registry '%s' Tools", registryName)
	if err := pageCreator.Run(title, tableContent); err != nil {
		return fmt.Errorf("failed to display output: %w", err)
	}

	// Show helpful hints after pager closes (so they're visible).
	_ = ui.Writeln("")
	_ = ui.Hintf("Use `atmos toolchain info <tool>` for details")
	_ = ui.Hintf("Use `atmos toolchain install <tool>@<version>` to install")

	return nil
}

func listRegistryTools(ctx context.Context, registryName string, opts *ListOptions) error {
	defer perf.Track(nil, "registry.listRegistryTools")()

	// Use pager with proper PagerSelector.
	pagerEnabled := opts.Pager.IsEnabled()

	// Create registry based on name.
	// For MVP, only support aqua-public.
	var reg toolchainregistry.ToolRegistry
	switch registryName {
	case "aqua-public", "aqua":
		reg = toolchain.NewAquaRegistry()
	default:
		return fmt.Errorf("%w: '%s' (supported registries: 'aqua-public', 'aqua')", toolchainregistry.ErrUnknownRegistry, registryName)
	}

	// Get tools from registry.
	tools, err := reg.ListAll(ctx,
		toolchainregistry.WithListLimit(opts.Limit),
		toolchainregistry.WithListOffset(opts.Offset),
		toolchainregistry.WithSort(opts.Sort),
	)
	if err != nil {
		return fmt.Errorf("failed to list tools: %w", err)
	}

	if len(tools) == 0 {
		_ = ui.Infof("No tools found in registry '%s'", registryName)
		return nil
	}

	// Output based on format.
	switch opts.Format {
	case "json":
		return data.WriteJSON(tools)
	case "yaml":
		return data.WriteYAML(tools)
	case "table":
		return displayToolsTable(ctx, reg, tools, registryName, opts, pagerEnabled)
	default:
		// Should never reach here due to validation in parseListOptions.
		return fmt.Errorf("%w: unsupported format: %s", errUtils.ErrInvalidFlag, opts.Format)
	}
}

func buildToolsTable(tools []*toolchainregistry.Tool) string {
	defer perf.Track(nil, "registry.buildToolsTable")()

	// Get terminal width.
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width == 0 {
		width = defaultTerminalWidth
	}

	// Load tool versions to check installation status.
	installer := toolchain.NewInstaller()
	toolVersionsFile := toolchain.GetToolVersionsFilePath()
	toolVersions, err := toolchain.LoadToolVersions(toolVersionsFile)
	if err != nil && !os.IsNotExist(err) {
		// If there's an error other than file not found, log it but continue.
		_ = ui.Warningf("Could not load .tool-versions: %v", err)
	}

	// Build row data with installation status.
	var rows []toolRow
	statusWidth := 1 // For single dot character.
	ownerWidth := len("OWNER")
	repoWidth := len("REPO")
	typeWidth := len("TYPE")

	// Check each tool's installation and configuration status.
	for _, tool := range tools {
		row := toolRow{
			owner:    tool.RepoOwner,
			repo:     tool.RepoName,
			toolType: tool.Type,
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
		switch {
		case row.isInstalled:
			row.status = statusIndicator // Green dot (will be colored later).
		case row.isInConfig:
			row.status = statusIndicator // Gray dot (will be colored later).
		default:
			row.status = " " // No indicator.
		}

		// Update column widths.
		if len(tool.RepoOwner) > ownerWidth {
			ownerWidth = len(tool.RepoOwner)
		}
		if len(tool.RepoName) > repoWidth {
			repoWidth = len(tool.RepoName)
		}
		if len(tool.Type) > typeWidth {
			typeWidth = len(tool.Type)
		}

		rows = append(rows, row)
	}

	// Add padding.
	const statusPadding = 2 // Minimal padding for status column (1 char + 1 space on right)
	ownerWidth += totalColumnPadding
	repoWidth += totalColumnPadding
	typeWidth += totalColumnPadding
	statusWidth += statusPadding

	// Calculate total width needed.
	totalNeededWidth := statusWidth + ownerWidth + repoWidth + typeWidth

	// If screen is narrow, truncate columns proportionally.
	if totalNeededWidth > width {
		excess := totalNeededWidth - width

		// Truncate proportionally, but keep minimums.
		ownerReduce := min(excess*3/10, ownerWidth-minColumnWidthOwner)
		repoReduce := min(excess*5/10, repoWidth-minColumnWidthRepo)
		typeReduce := min(excess*2/10, typeWidth-minColumnWidthType)

		ownerWidth -= ownerReduce
		repoWidth -= repoReduce
		typeWidth -= typeReduce
	}

	// Create table columns with calculated widths.
	columns := []table.Column{
		{Title: " ", Width: statusWidth}, // Status column.
		{Title: "OWNER", Width: ownerWidth},
		{Title: "REPO", Width: repoWidth},
		{Title: "TYPE", Width: typeWidth},
	}

	// Convert rows to table format.
	var tableRows []table.Row
	for _, row := range rows {
		tableRows = append(tableRows, table.Row{
			row.status,
			row.owner,
			row.repo,
			row.toolType,
		})
	}

	// Create and configure table.
	// Height = number of data rows + 1 for header row.
	tableHeight := len(tableRows) + 1
	t := table.New(
		table.WithColumns(columns),
		table.WithRows(tableRows),
		table.WithFocused(false),
		table.WithHeight(tableHeight),
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
	return renderTableWithConditionalStyling(t.View(), rows)
}

// min returns the smaller of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ListCommandProvider implements the CommandProvider interface for the 'toolchain registry list' command, wiring the list subcommand into the CLI framework with its associated flags and behaviors.
type ListCommandProvider struct{}

func (l *ListCommandProvider) GetCommand() *cobra.Command {
	return listCmd
}

func (l *ListCommandProvider) GetName() string {
	return "list"
}

func (l *ListCommandProvider) GetGroup() string {
	return "Toolchain Commands"
}

func (l *ListCommandProvider) GetFlagsBuilder() flags.Builder {
	return listParser
}

func (l *ListCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (l *ListCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}
