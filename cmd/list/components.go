package list

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/extract"
	"github.com/cloudposse/atmos/pkg/list/filter"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	listSort "github.com/cloudposse/atmos/pkg/list/sort"
	log "github.com/cloudposse/atmos/pkg/logger"
	perf "github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// Query normalization constants.
const (
	querySelectPrefix = "select("
	queryArraySuffix  = "]"
)

var componentsParser *flags.StandardParser

// ComponentsOptions contains parsed flags for the components command.
type ComponentsOptions struct {
	global.Flags
	Stack    string
	Type     string
	Enabled  *bool
	Locked   *bool
	Format   string
	Columns  []string
	Sort     string
	Abstract bool
	Query    string
}

// componentsCmd lists atmos components.
var componentsCmd = &cobra.Command{
	Use:   "components",
	Short: "List all Atmos components with filtering, sorting, and formatting options",
	Long:  `List Atmos components with support for filtering by stack, type, enabled/locked status, custom column selection, sorting, and multiple output formats.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get Viper instance for flag/env precedence.
		v := viper.GetViper()

		// Check Atmos configuration (honors --base-path, --config, --config-path, --profile).
		if err := checkAtmosConfig(cmd, v); err != nil {
			return err
		}

		// Parse flags using StandardParser with Viper precedence.
		if err := componentsParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Parse enabled/locked flags as tri-state (*bool).
		// nil = unset (show all), true = filter for true, false = filter for false.
		// Use cmd.Flags().Changed() instead of v.IsSet() because IsSet returns true
		// when a default value is registered, but we only want to filter when
		// the user explicitly provided the flag.
		var enabledPtr *bool
		if cmd.Flags().Changed("enabled") {
			val := v.GetBool("enabled")
			enabledPtr = &val
		}
		var lockedPtr *bool
		if cmd.Flags().Changed("locked") {
			val := v.GetBool("locked")
			lockedPtr = &val
		}

		opts := &ComponentsOptions{
			Flags:    flags.ParseGlobalFlags(cmd, v),
			Stack:    v.GetString("stack"),
			Type:     v.GetString("type"),
			Enabled:  enabledPtr,
			Locked:   lockedPtr,
			Format:   v.GetString("format"),
			Columns:  v.GetStringSlice("columns"),
			Sort:     v.GetString("sort"),
			Abstract: v.GetBool("abstract"),
			Query:    v.GetString("query"),
		}

		return listComponentsWithOptions(cmd, args, opts)
	},
}

// columnsCompletionForComponents provides dynamic tab completion for --columns flag.
// Returns column names from atmos.yaml components.list.columns configuration.
func columnsCompletionForComponents(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	defer perf.Track(nil, "list.components.columnsCompletionForComponents")()

	// Load atmos configuration with CLI flags.
	configAndStacksInfo, err := e.ProcessCommandLineArgs("list", cmd, args, nil)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, false)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Extract column names from atmos.yaml configuration.
	if len(atmosConfig.Components.List.Columns) > 0 {
		var columnNames []string
		for _, col := range atmosConfig.Components.List.Columns {
			columnNames = append(columnNames, col.Name)
		}
		return columnNames, cobra.ShellCompDirectiveNoFileComp
	}

	// If no custom columns configured, return empty list.
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	// Create parser with components-specific flags using flag wrappers.
	componentsParser = NewListParser(
		WithFormatFlag,
		WithComponentsColumnsFlag,
		WithSortFlag,
		WithStackFlag,
		WithTypeFlag,
		WithEnabledFlag,
		WithLockedFlag,
		WithAbstractFlag,
		WithQueryFlag,
	)

	// Register flags.
	componentsParser.RegisterFlags(componentsCmd)

	// Register dynamic tab completion for --columns flag.
	if err := componentsCmd.RegisterFlagCompletionFunc("columns", columnsCompletionForComponents); err != nil {
		panic(err)
	}

	// Bind flags to Viper for environment variable support.
	if err := componentsParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func listComponentsWithOptions(cmd *cobra.Command, args []string, opts *ComponentsOptions) error {
	defer perf.Track(nil, "list.components.listComponentsWithOptions")()

	// Initialize configuration and extract components.
	atmosConfig, components, err := initAndExtractComponents(cmd, args, opts)
	if err != nil {
		return err
	}

	if len(components) == 0 {
		_ = ui.Info("No components found")
		return nil
	}

	// Build and execute render pipeline.
	return renderComponents(atmosConfig, opts, components)
}

// initAndExtractComponents initializes config and extracts components from stacks.
func initAndExtractComponents(cmd *cobra.Command, args []string, opts *ComponentsOptions) (*schema.AtmosConfiguration, []map[string]any, error) {
	defer perf.Track(nil, "list.components.initAndExtractComponents")()

	// Process command line args to get ConfigAndStacksInfo with CLI flags.
	configAndStacksInfo, err := e.ProcessCommandLineArgs("list", cmd, args, nil)
	if err != nil {
		return nil, nil, err
	}

	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", errUtils.ErrInitializingCLIConfig, err)
	}

	// If format is empty, check command-specific config.
	if opts.Format == "" && atmosConfig.Components.List.Format != "" {
		opts.Format = atmosConfig.Components.List.Format
	}

	// Create AuthManager for authentication support.
	authManager, err := createAuthManagerForList(cmd, &atmosConfig)
	if err != nil {
		return nil, nil, err
	}

	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, authManager)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", errUtils.ErrExecuteDescribeStacks, err)
	}

	// Extract components into structured data.
	components, err := extract.Components(stacksMap)
	if err != nil {
		return nil, nil, err
	}

	return &atmosConfig, components, nil
}

// renderComponents builds the render pipeline and renders components.
func renderComponents(atmosConfig *schema.AtmosConfiguration, opts *ComponentsOptions, components []map[string]any) error {
	defer perf.Track(nil, "list.components.renderComponents")()

	// If --query is provided, filter components using YQ expression first.
	if opts.Query != "" {
		// Normalize the query to ensure it returns an array and handle simplified syntax.
		normalizedQuery, err := normalizeComponentQuery(opts.Query)
		if err != nil {
			return err
		}

		filtered, err := filterComponentsWithQuery(atmosConfig, components, normalizedQuery)
		if err != nil {
			return fmt.Errorf("query filter failed: %w", err)
		}
		components = filtered
	}

	// Build filters.
	filters := buildComponentFilters(opts)

	// Get column configuration.
	columns := getComponentColumns(atmosConfig, opts.Columns)

	// Build column selector.
	selector, err := column.NewSelector(columns, column.BuildColumnFuncMap())
	if err != nil {
		return fmt.Errorf("error creating column selector: %w", err)
	}

	// Build sorters.
	sorters, err := buildComponentSorters(opts.Sort)
	if err != nil {
		return fmt.Errorf("error parsing sort specification: %w", err)
	}

	// Create renderer and execute pipeline with pager support.
	outputFormat := format.Format(opts.Format)
	r := renderer.New(filters, selector, sorters, outputFormat, "")

	return renderWithPager(atmosConfig, "List Components", r, components)
}

// normalizeComponentQuery ensures the query returns an array of component maps.
// This function auto-wraps queries to prevent multi-document YAML parsing errors
// and provides a simplified syntax where users can write:
//
//	select(.locked == true)
//
// Instead of the verbose:
//
//	[.[] | select(.locked == true)]
//
// Returns error for scalar extraction queries that don't make sense for list components.
func normalizeComponentQuery(query string) (string, error) {
	defer perf.Track(nil, "list.components.normalizeComponentQuery")()

	originalQuery := query
	query = strings.TrimSpace(query)
	if query == "" {
		return query, nil
	}

	// Already wrapped in array brackets - use as-is.
	if strings.HasPrefix(query, "[") && strings.HasSuffix(query, "]") {
		return query, nil
	}

	// Identity query - returns all components.
	if query == "." {
		return query, nil
	}

	// Detect scalar extraction patterns (not supported for list components).
	if isScalarExtractionQuery(query) {
		return "", fmt.Errorf("%w: '%s'; use 'atmos describe component --query' for field extraction, "+
			"or use 'select(...)' to filter components", errUtils.ErrScalarExtractionNotSupported, query)
	}

	var normalized string

	// Normalize query based on its prefix.
	switch {
	case strings.HasPrefix(query, querySelectPrefix):
		// Starts with select() - add .[] | prefix and wrap in array.
		normalized = "[.[] | " + query + queryArraySuffix
	case strings.HasPrefix(query, ".[]"):
		// Starts with .[] - wrap in array to prevent multi-doc YAML.
		normalized = "[" + query + queryArraySuffix
	default:
		// Default: wrap in array.
		normalized = "[" + query + queryArraySuffix
	}

	log.Trace("Normalized component query", "original", originalQuery, "normalized", normalized)
	return normalized, nil
}

// isScalarExtractionQuery detects queries that extract scalar values instead of filtering.
// These queries are not supported for `list components` because they would return
// individual field values rather than complete component maps.
func isScalarExtractionQuery(query string) bool {
	defer perf.Track(nil, "list.components.isScalarExtractionQuery")()

	// .[].field - extracts field from each item (without select).
	if strings.HasPrefix(query, ".[].") && !strings.Contains(query, querySelectPrefix) {
		return true
	}

	// .[] | .field - extracts field with pipe (without select).
	if strings.HasPrefix(query, ".[] | .") && !strings.Contains(query, querySelectPrefix) {
		return true
	}

	// .[0].field or .[N].field - extracts from indexed item.
	if strings.HasPrefix(query, ".[") && strings.Contains(query, "].") {
		afterBracket := strings.SplitN(query, "].", 2)
		if len(afterBracket) == 2 && !strings.Contains(afterBracket[1], querySelectPrefix) {
			return true
		}
	}

	return false
}

// filterComponentsWithQuery filters components using a YQ expression.
// The query should be a YQ filter expression that selects matching components.
// Returns the filtered list of components to be rendered normally.
func filterComponentsWithQuery(atmosConfig *schema.AtmosConfiguration, components []map[string]any, query string) ([]map[string]any, error) {
	defer perf.Track(nil, "list.components.filterComponentsWithQuery")()

	// Use the existing YQ evaluation from pkg/utils.
	result, err := u.EvaluateYqExpression(atmosConfig, components, query)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate query '%s': %w", query, err)
	}

	// Handle the result - it could be a slice or a single item.
	switch v := result.(type) {
	case []any:
		// Convert []any back to []map[string]any.
		filtered := make([]map[string]any, 0, len(v))
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				filtered = append(filtered, m)
			}
		}
		return filtered, nil
	case []map[string]any:
		return v, nil
	case map[string]any:
		// Single result, wrap in slice.
		return []map[string]any{v}, nil
	case nil:
		// No results.
		return []map[string]any{}, nil
	default:
		return nil, fmt.Errorf("%w: got %T, expected array of components", errUtils.ErrQueryUnexpectedResultType, result)
	}
}

// buildComponentFilters creates filters based on command options.
func buildComponentFilters(opts *ComponentsOptions) []filter.Filter {
	defer perf.Track(nil, "list.components.buildComponentFilters")()

	var filters []filter.Filter

	// Stack filter (glob pattern).
	if opts.Stack != "" {
		globFilter, err := filter.NewGlobFilter("stack", opts.Stack)
		if err != nil {
			_ = ui.Warning(fmt.Sprintf("Invalid glob pattern '%s': %v, filter will be ignored", opts.Stack, err))
		} else {
			filters = append(filters, globFilter)
		}
	}

	// Type filter (authoritative when provided, targets type field: real/abstract).
	if opts.Type != "" && opts.Type != "all" {
		filters = append(filters, filter.NewColumnFilter("type", opts.Type))
	} else if opts.Type == "" && !opts.Abstract {
		// Only apply default abstract filter when Type is not set.
		filters = append(filters, filter.NewColumnFilter("type", "real"))
	}

	// Enabled filter (tri-state: nil = all, true = enabled only, false = disabled only).
	if opts.Enabled != nil {
		filters = append(filters, filter.NewBoolFilter("enabled", opts.Enabled))
	}

	// Locked filter (tri-state: nil = all, true = locked only, false = unlocked only).
	if opts.Locked != nil {
		filters = append(filters, filter.NewBoolFilter("locked", opts.Locked))
	}

	return filters
}

// getComponentColumns returns column configuration.
func getComponentColumns(atmosConfig *schema.AtmosConfiguration, columnsFlag []string) []column.Config {
	defer perf.Track(nil, "list.components.getComponentColumns")()

	// If --columns flag is provided, parse it and return.
	if len(columnsFlag) > 0 {
		return parseColumnsFlag(columnsFlag)
	}

	// Check atmos.yaml for components.list.columns configuration.
	if len(atmosConfig.Components.List.Columns) > 0 {
		var configs []column.Config
		for _, col := range atmosConfig.Components.List.Columns {
			configs = append(configs, column.Config{
				Name:  col.Name,
				Value: col.Value,
				Width: col.Width,
			})
		}
		return configs
	}

	// Default columns: show status dot and standard component fields.
	return []column.Config{
		{Name: " ", Value: "{{ .status }}", Width: 1},
		{Name: "Component", Value: "{{ .component }}"},
		{Name: "Stack", Value: "{{ .stack }}"},
		{Name: "Kind", Value: "{{ .kind }}"},           // terraform, helmfile, packer
		{Name: "Type", Value: "{{ .type }}"},           // real, abstract
		{Name: "Path", Value: "{{ .component_path }}"}, // filesystem path to component
	}
}

// buildComponentSorters creates sorters from sort specification.
func buildComponentSorters(sortSpec string) ([]*listSort.Sorter, error) {
	defer perf.Track(nil, "list.components.buildComponentSorters")()

	if sortSpec == "" {
		// Default sort: by component ascending.
		return []*listSort.Sorter{
			listSort.NewSorter("Component", listSort.Ascending),
		}, nil
	}

	return listSort.ParseSortSpec(sortSpec)
}

// parseColumnsFlag parses column specifications from CLI flag.
// Supports two formats:
//   - Simple field name: "component" → Name: "component", Value: "{{ .component }}"
//   - Named column with template: "Name=template" → Name: "Name", Value: "template"
//
// Examples:
//
//	--columns component,stack,type
//	--columns "Component={{ .component }},Stack={{ .stack }}"
//	--columns component --columns stack
func parseColumnsFlag(columnsFlag []string) []column.Config {
	defer perf.Track(nil, "list.components.parseColumnsFlag")()

	var configs []column.Config

	for _, spec := range columnsFlag {
		cfg := parseColumnSpec(spec)
		if cfg.Name != "" {
			configs = append(configs, cfg)
		}
	}

	return configs
}

// parseColumnSpec parses a single column specification.
// Format: "name" or "Name=template".
func parseColumnSpec(spec string) column.Config {
	defer perf.Track(nil, "list.components.parseColumnSpec")()

	spec = strings.TrimSpace(spec)
	if spec == "" {
		return column.Config{}
	}

	// Check for Name=template format.
	if idx := strings.Index(spec, "="); idx > 0 {
		name := strings.TrimSpace(spec[:idx])
		value := strings.TrimSpace(spec[idx+1:])

		// If value doesn't contain template syntax, wrap it.
		if !strings.Contains(value, "{{") {
			value = "{{ ." + value + " }}"
		}

		return column.Config{
			Name:  name,
			Value: value,
		}
	}

	// Simple field name: auto-generate template.
	// Use title case for display name.
	name := strings.Title(spec) //nolint:staticcheck // strings.Title is deprecated but works for simple ASCII column names
	value := "{{ ." + spec + " }}"

	return column.Config{
		Name:  name,
		Value: value,
	}
}
