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
	perf "github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
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

	// Create renderer and execute pipeline.
	outputFormat := format.Format(opts.Format)
	r := renderer.New(filters, selector, sorters, outputFormat, "")

	return r.Render(components)
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

	// Type filter (authoritative when provided, targets component_type field).
	if opts.Type != "" && opts.Type != "all" {
		filters = append(filters, filter.NewColumnFilter("component_type", opts.Type))
	} else if opts.Type == "" && !opts.Abstract {
		// Only apply default abstract filter when Type is not set.
		filters = append(filters, filter.NewColumnFilter("component_type", "real"))
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

	// Default columns: show all standard component fields.
	return []column.Config{
		{Name: "Component", Value: "{{ .component }}"},
		{Name: "Stack", Value: "{{ .stack }}"},
		{Name: "Type", Value: "{{ .type }}"},
		{Name: "Component Type", Value: "{{ .component_type }}"},
		{Name: "Enabled", Value: "{{ .enabled }}"},
		{Name: "Locked", Value: "{{ .locked }}"},
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
