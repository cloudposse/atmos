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
	defaultListLimit = 50
	columnWidthOwner = 20
	columnWidthRepo  = 25
	columnWidthType  = 15
)

var listParser *flags.StandardParser

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

	// List tools from specific registry.
	registryName := args[0]
	return listRegistryTools(ctx, registryName)
}

func listConfiguredRegistries(_ context.Context) error {
	defer perf.Track(nil, "registry.listConfiguredRegistries")()

	// For MVP, show placeholder message.
	// TODO: Load registries from atmos.yaml configuration.
	message := `**Configured registries:**

Registry configuration from atmos.yaml:
  - aqua-public (aqua, priority: 10)

Use 'atmos toolchain registry list <name>' to see tools in a registry
`
	_ = ui.MarkdownMessage(message)

	return nil
}

func listRegistryTools(ctx context.Context, registryName string) error {
	defer perf.Track(nil, "registry.listRegistryTools")()

	// Get flag values from Viper.
	v := viper.GetViper()
	listLimit := v.GetInt("limit")
	listOffset := v.GetInt("offset")
	listSort := v.GetString("sort")
	listFormat := strings.ToLower(v.GetString("format"))

	// Validate format flag.
	switch listFormat {
	case "table", "json", "yaml":
		// Valid formats.
	default:
		return fmt.Errorf("%w: format must be one of: table, json, yaml (got: %s)",
			errUtils.ErrInvalidFlag, listFormat)
	}

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
		toolchainregistry.WithListLimit(listLimit),
		toolchainregistry.WithListOffset(listOffset),
		toolchainregistry.WithSort(listSort),
	)
	if err != nil {
		return fmt.Errorf("failed to list tools: %w", err)
	}

	if len(tools) == 0 {
		_ = ui.Infof("No tools found in registry '%s'", registryName)
		return nil
	}

	// Output based on format.
	switch listFormat {
	case "json":
		return data.WriteJSON(tools)
	case "yaml":
		return data.WriteYAML(tools)
	case "table":
		// Get metadata for header.
		meta, err := reg.GetMetadata(ctx)
		if err == nil {
			header := fmt.Sprintf("**Tools in registry '%s'** (showing %d):\n\nType: %s\nSource: %s\n\n",
				registryName, len(tools), meta.Type, meta.Source)
			_ = ui.MarkdownMessage(header)
		}

		// Display as table.
		displayToolsTable(tools)
		return nil
	default:
		// Should never reach here due to validation above.
		return fmt.Errorf("%w: unsupported format: %s", errUtils.ErrInvalidFlag, listFormat)
	}
}

func displayToolsTable(tools []*toolchainregistry.Tool) {
	defer perf.Track(nil, "registry.displayToolsTable")()

	// Create table columns.
	columns := []table.Column{
		{Title: "OWNER", Width: columnWidthOwner},
		{Title: "REPO", Width: columnWidthRepo},
		{Title: "TYPE", Width: columnWidthType},
	}

	// Convert tools to table rows.
	var rows []table.Row
	for _, tool := range tools {
		rows = append(rows, table.Row{
			tool.RepoOwner,
			tool.RepoName,
			tool.Type,
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
	_ = data.Writeln(t.View())
}

// ListCommandProvider implements the CommandProvider interface.
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
