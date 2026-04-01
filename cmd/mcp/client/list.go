package client

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/mcp/mcpcmd"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/filter"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	listSort "github.com/cloudposse/atmos/pkg/list/sort"
	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

//go:embed markdown/atmos_mcp_list.md
var listLongMarkdown string

// mcpListParser handles flag parsing for the mcp list command.
var mcpListParser *flags.StandardParser

// MCPListOptions contains parsed flags for the mcp list command.
type MCPListOptions struct {
	Format    string
	Columns   []string
	Sort      string
	Delimiter string
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured MCP servers",
	Long:  listLongMarkdown,
	Args:  cobra.NoArgs,
	RunE:  executeMCPList,
}

const (
	flagFormat    = "format"
	flagColumns   = "columns"
	flagSort      = "sort"
	flagDelimiter = "delimiter"
)

func init() {
	// Create parser with format, columns, sort, and delimiter flags.
	mcpListParser = flags.NewStandardParser(
		flags.WithStringFlag(flagFormat, "f", "", "Output format: table, json, yaml, csv, tsv"),
		flags.WithEnvVars(flagFormat, "ATMOS_LIST_FORMAT"),
		flags.WithValidValues(flagFormat, "table", "json", "yaml", "csv", "tsv"),
		flags.WithStringSliceFlag(flagColumns, "", []string{}, "Columns to display (comma-separated, overrides defaults)"),
		flags.WithEnvVars(flagColumns, "ATMOS_LIST_COLUMNS"),
		flags.WithStringFlag(flagSort, "", "", "Sort by column:order (e.g., 'name:asc,status:desc')"),
		flags.WithEnvVars(flagSort, "ATMOS_LIST_SORT"),
		flags.WithStringFlag(flagDelimiter, "", "", "Delimiter for CSV/TSV output"),
		flags.WithEnvVars(flagDelimiter, "ATMOS_LIST_DELIMITER"),
	)

	// Register flags on the command.
	mcpListParser.RegisterFlags(listCmd)

	// Bind flags to Viper for environment variable support.
	if err := mcpListParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	mcpcmd.McpCmd.AddCommand(listCmd)
}

func executeMCPList(cmd *cobra.Command, _ []string) error {
	defer perf.Track(nil, "cmd.mcpList")()

	// Parse flags using StandardParser with Viper precedence.
	v := viper.GetViper()
	if err := mcpListParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	opts := &MCPListOptions{
		Format:    v.GetString(flagFormat),
		Columns:   v.GetStringSlice(flagColumns),
		Sort:      v.GetString(flagSort),
		Delimiter: v.GetString(flagDelimiter),
	}

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}

	if len(atmosConfig.MCP.Servers) == 0 {
		ui.Info("No MCP servers configured. Add servers under `mcp.servers` in `atmos.yaml`.")
		return nil
	}

	mgr, err := mcpclient.NewManager(atmosConfig.MCP.Servers)
	if err != nil {
		return err
	}

	// Build structured data for the renderer pipeline.
	sessions := mgr.List()
	data := make([]map[string]any, 0, len(sessions))
	for _, session := range sessions {
		data = append(data, map[string]any{
			"name":        session.Name(),
			"status":      string(session.Status()),
			"description": session.Config().Description,
		})
	}

	return renderMCPList(data, opts)
}

// renderMCPList runs the standard renderer pipeline for mcp list output.
func renderMCPList(data []map[string]any, opts *MCPListOptions) error {
	defer perf.Track(nil, "cmd.mcpList.renderMCPList")()

	// No filters needed for mcp list (servers are already enumerated).
	var filters []filter.Filter

	// Get column configuration.
	cols := getMCPListColumns(opts.Columns)

	// Build column selector.
	selector, err := column.NewSelector(cols, column.BuildColumnFuncMap())
	if err != nil {
		return fmt.Errorf("error creating column selector: %w", err)
	}

	// Build sorters.
	sorters, err := buildMCPListSorters(opts.Sort)
	if err != nil {
		return fmt.Errorf("error parsing sort specification: %w", err)
	}

	outputFormat := format.Format(opts.Format)
	r := renderer.New(filters, selector, sorters, outputFormat, opts.Delimiter)

	return r.Render(data)
}

// getMCPListColumns returns column configuration from flag or defaults.
func getMCPListColumns(columnsFlag []string) []column.Config {
	if len(columnsFlag) > 0 {
		return parseMCPColumnsFlag(columnsFlag)
	}

	// Default columns: NAME, STATUS, DESCRIPTION.
	return []column.Config{
		{Name: "NAME", Value: "{{ .name }}"},
		{Name: "STATUS", Value: "{{ .status }}"},
		{Name: "DESCRIPTION", Value: "{{ .description }}"},
	}
}

// parseMCPColumnsFlag converts column flag strings to column.Config entries.
// Supports two formats:
//   - Simple name: "name" → {Name: "name", Value: "{{ .name }}"}
//   - Name=template: "Name={{ .name }}" → {Name: "Name", Value: "{{ .name }}"}
func parseMCPColumnsFlag(columnsFlag []string) []column.Config {
	var configs []column.Config
	for _, spec := range columnsFlag {
		if spec == "" {
			continue
		}
		cfg := parseMCPColumnSpec(spec)
		if cfg.Name != "" {
			configs = append(configs, cfg)
		}
	}
	return configs
}

// parseMCPColumnSpec parses a single column specification string.
// For a simple name like "NAME", the display header is the spec itself and the data key
// is the lowercase version (so "NAME" reads from .name in the template context).
// For "Label=template" format, the name and value are taken as-is.
func parseMCPColumnSpec(spec string) column.Config {
	for i, ch := range spec {
		if ch == '=' && i > 0 {
			name := spec[:i]
			value := spec[i+1:]
			if value != "" && !containsTemplate(value) {
				value = "{{ ." + value + " }}"
			}
			return column.Config{Name: name, Value: value}
		}
	}
	// Simple name — display header is the exact spec, data key is lowercased.
	// This matches the default column definitions where "NAME" reads from .name.
	dataKey := strings.ToLower(spec)
	return column.Config{
		Name:  spec,
		Value: "{{ ." + dataKey + " }}",
	}
}

// containsTemplate returns true if the string contains Go template syntax.
func containsTemplate(s string) bool {
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '{' && s[i+1] == '{' {
			return true
		}
	}
	return false
}

// buildMCPListSorters creates sorters from sort specification.
func buildMCPListSorters(sortSpec string) ([]*listSort.Sorter, error) {
	if sortSpec == "" {
		// Default: sort by name ascending.
		return []*listSort.Sorter{
			listSort.NewSorter("NAME", listSort.Ascending),
		}, nil
	}
	return listSort.ParseSortSpec(sortSpec)
}
