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
	listLimit  int
	listOffset int
	listFormat string
	listSort   string
)

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
	listCmd.Flags().IntVar(&listLimit, "limit", 50, "Maximum number of results to show")
	listCmd.Flags().IntVar(&listOffset, "offset", 0, "Skip first N results (pagination)")
	listCmd.Flags().StringVar(&listFormat, "format", "table", "Output format (table, json, yaml)")
	listCmd.Flags().StringVar(&listSort, "sort", "name", "Sort order (name, date, popularity)")
}

func executeListCommand(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "registry.executeListCommand")()

	ctx := context.Background()

	// If no registry name provided, list all configured registries.
	if len(args) == 0 {
		return listConfiguredRegistries(ctx)
	}

	// List tools from specific registry.
	registryName := args[0]
	return listRegistryTools(ctx, registryName)
}

func listConfiguredRegistries(ctx context.Context) error {
	defer perf.Track(nil, "registry.listConfiguredRegistries")()

	// Check context for cancellation.
	if err := ctx.Err(); err != nil {
		return err
	}

	// For MVP, show placeholder message.
	// TODO: Load registries from atmos.yaml configuration.
	u.PrintfMarkdownToTUI("**Configured registries:**\n\n")
	u.PrintfMessageToTUI("Registry configuration from atmos.yaml:\n")
	u.PrintfMessageToTUI("  - aqua-public (aqua, priority: 10)\n")
	u.PrintfMessageToTUI("\n")
	u.PrintfMessageToTUI("Use 'atmos toolchain registry list <name>' to see tools in a registry\n")

	return nil
}

func listRegistryTools(ctx context.Context, registryName string) error {
	defer perf.Track(nil, "registry.listRegistryTools")()

	// Create registry based on name.
	// For MVP, only support aqua-public.
	var reg toolchainregistry.ToolRegistry
	switch registryName {
	case "aqua-public", "aqua":
		reg = toolchain.NewAquaRegistry()
	default:
		return fmt.Errorf("%w: %s (supported: aqua-public)", toolchainregistry.ErrUnknownRegistry, registryName)
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
		u.PrintfMessageToTUI("No tools found in registry '%s'\n", registryName)
		return nil
	}

	// Get metadata for header.
	meta, err := reg.GetMetadata(ctx)
	if err == nil {
		u.PrintfMarkdownToTUI("**Tools in registry '%s'** (showing %d):\n\n", registryName, len(tools))
		u.PrintfMessageToTUI("Type: %s\nSource: %s\n\n", meta.Type, meta.Source)
	}

	// Display as table.
	displayToolsTable(tools)

	return nil
}

func displayToolsTable(tools []*toolchainregistry.Tool) {
	defer perf.Track(nil, "registry.displayToolsTable")()

	// Create table columns.
	columns := []table.Column{
		{Title: "OWNER", Width: 20},
		{Title: "REPO", Width: 25},
		{Title: "TYPE", Width: 15},
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
	fmt.Println(t.View())
}
