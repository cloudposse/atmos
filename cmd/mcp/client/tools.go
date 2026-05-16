package client

import (
	"context"
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

//go:embed markdown/atmos_mcp_tools.md
var toolsLongMarkdown string

// mcpToolsParser handles flag parsing for the mcp tools command.
var mcpToolsParser *flags.StandardParser

// MCPToolsOptions contains parsed flags for the mcp tools command.
type MCPToolsOptions struct {
	Format    string
	Columns   []string
	Sort      string
	Delimiter string
}

var toolsCmd = &cobra.Command{
	Use:   "tools <name>",
	Short: "List tools from an MCP server",
	Long:  toolsLongMarkdown,
	Args:  cobra.ExactArgs(1),
	RunE:  executeMCPTools,
}

func init() {
	// Match the flag surface of `mcp list` so users get the same DX:
	// --format / --columns / --sort / --delimiter, with the same env-var
	// fallbacks. This addresses issue #7 in
	// docs/fixes/2026-05-15-mcp-review-fixes.md.
	mcpToolsParser = flags.NewStandardParser(
		flags.WithStringFlag(flagFormat, "f", "", "Output format: table, json, yaml, csv, tsv"),
		flags.WithEnvVars(flagFormat, "ATMOS_LIST_FORMAT"),
		flags.WithValidValues(flagFormat, "table", "json", "yaml", "csv", "tsv"),
		flags.WithStringSliceFlag(flagColumns, "", []string{}, "Columns to display (comma-separated, overrides defaults)"),
		flags.WithEnvVars(flagColumns, "ATMOS_LIST_COLUMNS"),
		flags.WithStringFlag(flagSort, "", "", "Sort by column:order (e.g., 'name:asc')"),
		flags.WithEnvVars(flagSort, "ATMOS_LIST_SORT"),
		flags.WithStringFlag(flagDelimiter, "", "", "Delimiter for CSV/TSV output"),
		flags.WithEnvVars(flagDelimiter, "ATMOS_LIST_DELIMITER"),
	)

	mcpToolsParser.RegisterFlags(toolsCmd)
	if err := mcpToolsParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	mcpcmd.McpCmd.AddCommand(toolsCmd)
}

func executeMCPTools(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "cmd.mcpTools")()
	name := args[0]

	// Parse flags using StandardParser with Viper precedence.
	v := viper.GetViper()
	if err := mcpToolsParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}
	opts := &MCPToolsOptions{
		Format:    v.GetString(flagFormat),
		Columns:   v.GetStringSlice(flagColumns),
		Sort:      v.GetString(flagSort),
		Delimiter: v.GetString(flagDelimiter),
	}

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}

	mgr, err := mcpclient.NewManager(atmosConfig.MCP.Servers)
	if err != nil {
		return err
	}
	defer mgr.StopAll() //nolint:errcheck // Best-effort cleanup.

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	startOpts := buildStartOptions(&atmosConfig)
	if err := mgr.Start(ctx, name, startOpts...); err != nil {
		return err
	}

	session, err := mgr.Get(name)
	if err != nil {
		return err
	}

	tools := session.Tools()
	if len(tools) == 0 {
		ui.Info("No tools available from `" + name + "`")
		return nil
	}

	// Build structured data for the renderer pipeline. The `description`
	// field carries the truncated first sentence so the default table
	// format stays scannable; users who want the full description should
	// use `--format=json` (or yaml) for the raw value.
	data := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		data = append(data, map[string]any{
			"name":             tool.Name,
			"description":      firstSentence(tool.Description),
			"full_description": tool.Description,
		})
	}

	return renderMCPTools(data, opts)
}

// renderMCPTools runs the standard renderer pipeline for mcp tools output.
// Shape parallels renderMCPList in list.go — see that function for context
// on why the four-stage filter → column → sort → format pipeline is
// preferred over hand-rolled tables.
func renderMCPTools(data []map[string]any, opts *MCPToolsOptions) error {
	defer perf.Track(nil, "cmd.mcpTools.renderMCPTools")()

	// No filters needed: the data set is already scoped to the chosen
	// server's tools.
	var filters []filter.Filter

	cols := getMCPToolsColumns(opts.Columns)
	selector, err := column.NewSelector(cols, column.BuildColumnFuncMap())
	if err != nil {
		return fmt.Errorf("error creating column selector: %w", err)
	}

	sorters, err := buildMCPToolsSorters(opts.Sort)
	if err != nil {
		return fmt.Errorf("error parsing sort specification: %w", err)
	}

	outputFormat := format.Format(opts.Format)
	r := renderer.New(filters, selector, sorters, outputFormat, opts.Delimiter)

	return r.Render(data)
}

// getMCPToolsColumns returns column configuration from the flag or defaults.
//
// Defaults are intentionally minimal (NAME + DESCRIPTION) to match what the
// pre-renderer version of this command showed. Users who want the full
// untruncated description can either pass `--columns NAME,FULL_DESCRIPTION`
// or drop into a non-table format that includes the raw field.
func getMCPToolsColumns(columnsFlag []string) []column.Config {
	if len(columnsFlag) > 0 {
		return parseMCPToolsColumnsFlag(columnsFlag)
	}
	return []column.Config{
		{Name: "NAME", Value: "{{ .name }}"},
		{Name: "DESCRIPTION", Value: "{{ .description }}"},
	}
}

// parseMCPToolsColumnsFlag mirrors parseMCPColumnsFlag in list.go.
// Supports two forms:
//
//   - Simple name: "name" → {Name: "name", Value: "{{ .name }}"}
//   - Name=template: "Name={{ .name }}" → {Name: "Name", Value: "{{ .name }}"}
func parseMCPToolsColumnsFlag(columnsFlag []string) []column.Config {
	var configs []column.Config
	for _, spec := range columnsFlag {
		if spec == "" {
			continue
		}
		c := parseMCPToolsColumnSpec(spec)
		if c.Name != "" {
			configs = append(configs, c)
		}
	}
	return configs
}

// parseMCPToolsColumnSpec parses a single column specification string.
// Identical shape to parseMCPColumnSpec in list.go.
func parseMCPToolsColumnSpec(spec string) column.Config {
	for i, ch := range spec {
		if ch == '=' && i > 0 {
			name := spec[:i]
			value := spec[i+1:]
			if value != "" && !mcpToolsContainsTemplate(value) {
				value = "{{ ." + value + " }}"
			}
			return column.Config{Name: name, Value: value}
		}
	}
	dataKey := strings.ToLower(spec)
	return column.Config{
		Name:  spec,
		Value: "{{ ." + dataKey + " }}",
	}
}

// mcpToolsContainsTemplate is a renamed local copy of list.go's
// containsTemplate so the two commands stay independent of each other's
// implementation. The body is intentionally trivial.
func mcpToolsContainsTemplate(s string) bool {
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '{' && s[i+1] == '{' {
			return true
		}
	}
	return false
}

// buildMCPToolsSorters creates sorters from a sort spec, defaulting to
// NAME ascending (most useful for browsing).
func buildMCPToolsSorters(sortSpec string) ([]*listSort.Sorter, error) {
	if sortSpec == "" {
		return []*listSort.Sorter{
			listSort.NewSorter("NAME", listSort.Ascending),
		}, nil
	}
	return listSort.ParseSortSpec(sortSpec)
}

// firstSentenceMaxLen is the hard upper bound for firstSentence output.
// Tool descriptions that don't contain an explicit sentence terminator (or
// have a terminator past this column) are truncated with an ellipsis.
const firstSentenceMaxLen = 80

// firstSentence extracts the first sentence from a description, collapsing
// whitespace, and ensures the output never exceeds firstSentenceMaxLen runes.
//
// Recognized sentence terminators are `. `, `! `, `? ` — matching standard
// English punctuation. A markdown header boundary (` ##`) is also treated
// as a sentence end (turning the preceding text into a single sentence).
//
// If no terminator is found, the input is hard-truncated to
// firstSentenceMaxLen with an ellipsis. Earlier versions only split on
// `. ` / ` ##` and applied no length bound — descriptions ending in `!`,
// `?`, or with no terminator at all leaked full paragraphs into the
// `atmos mcp tools` table.
func firstSentence(desc string) string {
	// Collapse all whitespace (newlines, tabs, multiple spaces) into single spaces.
	desc = strings.Join(strings.Fields(desc), " ")
	if desc == "" {
		return ""
	}

	// Find the earliest sentence terminator. `strings.IndexAny` doesn't help
	// here because we need terminator + space (so we don't split on a period
	// inside a version string like "v1.0" or inside an abbreviation).
	candidates := []string{". ", "! ", "? "}
	earliest := -1
	for _, t := range candidates {
		if idx := strings.Index(desc, t); idx > 0 && (earliest < 0 || idx < earliest) {
			earliest = idx
		}
	}
	if earliest > 0 {
		// desc[:earliest+1] includes the terminator's punctuation but not
		// the following space.
		return desc[:earliest+1]
	}

	// Stop at markdown header even when no sentence terminator was found.
	if idx := strings.Index(desc, " ##"); idx > 0 {
		return strings.TrimSpace(desc[:idx]) + "."
	}

	// No terminator and no markdown header — apply the hard length bound.
	// strings.Fields above already collapsed runs of whitespace, so this is
	// just a length check on the collapsed string.
	if len(desc) > firstSentenceMaxLen {
		const ellipsis = "…"
		runes := []rune(desc)
		if len(runes) > firstSentenceMaxLen {
			return string(runes[:firstSentenceMaxLen-1]) + ellipsis
		}
	}

	return desc
}
