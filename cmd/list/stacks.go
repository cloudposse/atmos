package list

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/extract"
	"github.com/cloudposse/atmos/pkg/list/filter"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/importresolver"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	listSort "github.com/cloudposse/atmos/pkg/list/sort"
	"github.com/cloudposse/atmos/pkg/list/tree"
	log "github.com/cloudposse/atmos/pkg/logger"
	perf "github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

var stacksParser *flags.StandardParser

// StacksOptions contains parsed flags for the stacks command.
type StacksOptions struct {
	global.Flags
	Component  string
	Format     string
	Columns    []string
	Sort       string
	Provenance bool
}

// stacksCmd lists atmos stacks.
var stacksCmd = &cobra.Command{
	Use:   "stacks",
	Short: "List all Atmos stacks with filtering, sorting, and formatting options",
	Long:  `List Atmos stacks with support for filtering by component, custom column selection, sorting, and multiple output formats.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration.
		if err := checkAtmosConfig(); err != nil {
			return err
		}

		// Parse flags using StandardParser with Viper precedence.
		v := viper.GetViper()
		if err := stacksParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := &StacksOptions{
			Flags:      flags.ParseGlobalFlags(cmd, v),
			Component:  v.GetString("component"),
			Format:     v.GetString("format"),
			Columns:    v.GetStringSlice("columns"),
			Sort:       v.GetString("sort"),
			Provenance: v.GetBool("provenance"),
		}

		return listStacksWithOptions(cmd, args, opts)
	},
}

// columnsCompletionForStacks provides dynamic tab completion for --columns flag.
// Returns column names from atmos.yaml stacks.list.columns configuration.
func columnsCompletionForStacks(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	defer perf.Track(nil, "list.stacks.columnsCompletionForStacks")()

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
	if len(atmosConfig.Stacks.List.Columns) > 0 {
		var columnNames []string
		for _, col := range atmosConfig.Stacks.List.Columns {
			columnNames = append(columnNames, col.Name)
		}
		return columnNames, cobra.ShellCompDirectiveNoFileComp
	}

	// If no custom columns configured, return empty list.
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	// Create parser with stacks-specific flags using flag wrappers.
	stacksParser = NewListParser(
		WithFormatFlag,
		WithStacksColumnsFlag,
		WithSortFlag,
		WithComponentFlag,
		WithProvenanceFlag,
	)

	// Register flags.
	stacksParser.RegisterFlags(stacksCmd)

	// Register dynamic tab completion for --columns flag.
	if err := stacksCmd.RegisterFlagCompletionFunc("columns", columnsCompletionForStacks); err != nil {
		panic(err)
	}

	// Bind flags to Viper for environment variable support.
	if err := stacksParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func listStacksWithOptions(cmd *cobra.Command, args []string, opts *StacksOptions) error {
	defer perf.Track(nil, "list.stacks.listStacksWithOptions")()

	// Early validation: --provenance only works with --format=tree.
	if err := validateProvenanceFlag(opts); err != nil {
		return err
	}

	// Initialize configuration and auth.
	atmosConfig, authManager, err := initStacksConfig(cmd, args, opts)
	if err != nil {
		return err
	}

	// Execute describe stacks and extract results.
	stacks, stacksMap, err := executeAndExtractStacks(&atmosConfig, opts, authManager)
	if err != nil {
		return err
	}
	if len(stacks) == 0 {
		_ = ui.Info("No stacks found")
		return nil
	}

	// Handle tree format specially - it shows import hierarchies.
	if opts.Format == string(format.FormatTree) {
		return renderStacksTreeFormat(&atmosConfig, stacks, opts.Provenance, authManager)
	}
	_ = stacksMap // Unused in non-tree format.

	// Render stacks with filters, columns, and sorters.
	return renderStacksTable(&atmosConfig, stacks, opts)
}

// validateProvenanceFlag checks that --provenance is only used with --format=tree.
func validateProvenanceFlag(opts *StacksOptions) error {
	if opts.Provenance && opts.Format != "" && opts.Format != string(format.FormatTree) {
		return fmt.Errorf("%w: --provenance flag only works with --format=tree", errUtils.ErrInvalidFlag)
	}
	return nil
}

// initStacksConfig initializes configuration and authentication for the stacks command.
func initStacksConfig(
	cmd *cobra.Command,
	args []string,
	opts *StacksOptions,
) (schema.AtmosConfiguration, auth.AuthManager, error) {
	defer perf.Track(nil, "list.stacks.initStacksConfig")()

	configAndStacksInfo, err := e.ProcessCommandLineArgs("list", cmd, args, nil)
	if err != nil {
		return schema.AtmosConfiguration{}, nil, err
	}

	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return schema.AtmosConfiguration{}, nil, fmt.Errorf("%w: %w", errUtils.ErrInitializingCLIConfig, err)
	}

	// Apply format from config if not set via flag.
	if opts.Format == "" && atmosConfig.Stacks.List.Format != "" {
		opts.Format = atmosConfig.Stacks.List.Format
	}

	// Validate provenance after resolving format from config.
	if opts.Provenance && opts.Format != string(format.FormatTree) {
		return schema.AtmosConfiguration{}, nil, fmt.Errorf("%w: --provenance flag only works with --format=tree", errUtils.ErrInvalidFlag)
	}

	authManager, err := createAuthManagerForList(cmd, &atmosConfig)
	if err != nil {
		return schema.AtmosConfiguration{}, nil, err
	}

	return atmosConfig, authManager, nil
}

// executeAndExtractStacks runs describe stacks and extracts the results.
func executeAndExtractStacks(
	atmosConfig *schema.AtmosConfiguration,
	opts *StacksOptions,
	authManager auth.AuthManager,
) ([]map[string]any, map[string]any, error) {
	defer perf.Track(nil, "list.stacks.executeAndExtractStacks")()

	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, false, false, false, nil, authManager)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", errUtils.ErrExecuteDescribeStacks, err)
	}

	var stacks []map[string]any
	if opts.Component != "" {
		stacks, err = extract.StacksForComponent(opts.Component, stacksMap)
	} else {
		stacks, err = extract.Stacks(stacksMap)
	}
	if err != nil {
		return nil, nil, err
	}

	return stacks, stacksMap, nil
}

// renderStacksTable renders stacks in table format with filters, columns, and sorters.
func renderStacksTable(atmosConfig *schema.AtmosConfiguration, stacks []map[string]any, opts *StacksOptions) error {
	defer perf.Track(nil, "list.stacks.renderStacksTable")()

	filters := buildStackFilters(opts)
	columns := getStackColumns(atmosConfig, opts.Columns, opts.Component != "")

	selector, err := column.NewSelector(columns, column.BuildColumnFuncMap())
	if err != nil {
		return fmt.Errorf("error creating column selector: %w", err)
	}

	sorters, err := buildStackSorters(opts.Sort)
	if err != nil {
		return fmt.Errorf("error parsing sort specification: %w", err)
	}

	outputFormat := format.Format(opts.Format)
	r := renderer.New(filters, selector, sorters, outputFormat, "")
	return r.Render(stacks)
}

// buildStackFilters creates filters based on command options.
func buildStackFilters(opts *StacksOptions) []filter.Filter {
	var filters []filter.Filter

	// Component filter already handled by extraction logic.
	// Add any additional filters here in the future.

	return filters
}

// getStackColumns returns column configuration.
func getStackColumns(atmosConfig *schema.AtmosConfiguration, columnsFlag []string, hasComponent bool) []column.Config {
	defer perf.Track(nil, "list.stacks.getStackColumns")()

	// If --columns flag is provided, parse it and return.
	if len(columnsFlag) > 0 {
		return parseColumnsFlag(columnsFlag)
	}

	// Check atmos.yaml for stacks.list.columns configuration.
	if len(atmosConfig.Stacks.List.Columns) > 0 {
		var configs []column.Config
		for _, col := range atmosConfig.Stacks.List.Columns {
			configs = append(configs, column.Config{
				Name:  col.Name,
				Value: col.Value,
				Width: col.Width,
			})
		}
		return configs
	}

	// Default columns for stacks.
	if hasComponent {
		// When filtering by component, show both stack and component.
		return []column.Config{
			{Name: "Stack", Value: "{{ .stack }}"},
			{Name: "Component", Value: "{{ .component }}"},
		}
	}

	// When showing all stacks, just show stack name.
	return []column.Config{
		{Name: "Stack", Value: "{{ .stack }}"},
	}
}

// renderStacksTreeFormat handles the tree format output for stacks.
// It enables provenance tracking, re-processes stacks, and renders the import hierarchy.
func renderStacksTreeFormat(
	atmosConfig *schema.AtmosConfiguration,
	stacks []map[string]any,
	showProvenance bool,
	authManager auth.AuthManager,
) error {
	defer perf.Track(nil, "list.stacks.renderStacksTreeFormat")()

	log.Trace("Tree format detected, enabling provenance tracking")
	atmosConfig.TrackProvenance = true

	// Clear caches to ensure fresh processing with provenance enabled.
	e.ClearMergeContexts()
	e.ClearFindStacksMapCache()
	log.Trace("Caches cleared, re-processing with provenance")

	// Re-process stacks with provenance tracking enabled.
	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, false, false, false, nil, authManager)
	if err != nil {
		return fmt.Errorf("error re-processing stacks with provenance: %w", err)
	}

	// Resolve import trees and filter to allowed stacks.
	importTrees, err := resolveAndFilterImportTrees(stacksMap, atmosConfig, stacks)
	if err != nil {
		return err
	}

	// Render and output the tree.
	output := format.RenderStacksTree(importTrees, showProvenance)
	_ = data.Writeln(output)
	return nil
}

// resolveAndFilterImportTrees resolves import trees from provenance and filters to allowed stacks.
func resolveAndFilterImportTrees(
	stacksMap map[string]any,
	atmosConfig *schema.AtmosConfiguration,
	stacks []map[string]any,
) (map[string][]*tree.ImportNode, error) {
	defer perf.Track(nil, "list.stacks.resolveAndFilterImportTrees")()

	importTreesWithComponents, err := importresolver.ResolveImportTreeFromProvenance(stacksMap, atmosConfig)
	if err != nil {
		return nil, fmt.Errorf("error resolving import tree from provenance: %w", err)
	}

	// Build a set of allowed stack names from the already-filtered stacks slice.
	allowedStacks := buildAllowedStacksSet(stacks)

	// Flatten component level - for stacks view, we just need stack → imports.
	// All components in a stack share the same import chain from the stack file.
	importTrees := make(map[string][]*tree.ImportNode)
	for stackName, componentImports := range importTreesWithComponents {
		if !allowedStacks[stackName] {
			continue
		}
		// Just take the first component's imports (they're all the same for a stack file).
		for _, imports := range componentImports {
			importTrees[stackName] = imports
			break
		}
	}

	return importTrees, nil
}

// buildAllowedStacksSet creates a set of stack names from a slice of stack maps.
func buildAllowedStacksSet(stacks []map[string]any) map[string]bool {
	defer perf.Track(nil, "list.stacks.buildAllowedStacksSet")()

	allowedStacks := make(map[string]bool)
	for _, stack := range stacks {
		if stackName, ok := stack["stack"].(string); ok {
			allowedStacks[stackName] = true
		}
	}
	return allowedStacks
}

// buildStackSorters creates sorters from sort specification.
func buildStackSorters(sortSpec string) ([]*listSort.Sorter, error) {
	defer perf.Track(nil, "list.stacks.buildStackSorters")()

	if sortSpec == "" {
		// Default sort: by stack ascending.
		return []*listSort.Sorter{
			listSort.NewSorter("Stack", listSort.Ascending),
		}, nil
	}

	return listSort.ParseSortSpec(sortSpec)
}
