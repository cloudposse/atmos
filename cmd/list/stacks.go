package list

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/filter"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	listSort "github.com/cloudposse/atmos/pkg/list/sort"
	"github.com/cloudposse/atmos/pkg/list/tree"
	log "github.com/cloudposse/atmos/pkg/logger"
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

		return listStacksWithOptions(opts)
	},
}

// columnsCompletionForStacks provides dynamic tab completion for --columns flag.
// Returns column names from atmos.yaml stacks.list.columns configuration.
func columnsCompletionForStacks(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Load atmos configuration.
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
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

func listStacksWithOptions(opts *StacksOptions) error {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrInitializingCLIConfig, err)
	}

	// If format is empty, check command-specific config.
	if opts.Format == "" && atmosConfig.Stacks.List.Format != "" {
		opts.Format = atmosConfig.Stacks.List.Format
	}

	// Validate that --provenance only works with --format=tree (after resolving format from config).
	if opts.Provenance && opts.Format != string(format.FormatTree) {
		return fmt.Errorf("%w: --provenance flag only works with --format=tree", errUtils.ErrInvalidFlag)
	}

	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrExecuteDescribeStacks, err)
	}

	// Extract stacks into structured data.
	var stacks []map[string]any
	if opts.Component != "" {
		stacks, err = l.ExtractStacksForComponent(opts.Component, stacksMap)
		if err != nil {
			return err
		}
	} else {
		stacks, err = l.ExtractStacks(stacksMap)
		if err != nil {
			return err
		}
	}

	if len(stacks) == 0 {
		_ = ui.Info("No stacks found")
		return nil
	}

	// Handle tree format specially - it shows import hierarchies.
	if opts.Format == "tree" {
		log.Trace("Tree format detected, enabling provenance tracking")
		// Enable provenance tracking to capture import chains.
		atmosConfig.TrackProvenance = true

		// Clear caches to ensure fresh processing with provenance enabled.
		e.ClearMergeContexts()
		e.ClearFindStacksMapCache()
		log.Trace("Caches cleared, re-processing with provenance")

		// Re-process stacks with provenance tracking enabled.
		stacksMap, err = e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
		if err != nil {
			return fmt.Errorf("error re-processing stacks with provenance: %w", err)
		}

		// Resolve import trees using provenance system.
		importTreesWithComponents, err := l.ResolveImportTreeFromProvenance(stacksMap, &atmosConfig)
		if err != nil {
			return fmt.Errorf("error resolving import tree from provenance: %w", err)
		}

		// Flatten component level - for stacks view, we just need stack → imports.
		// All components in a stack share the same import chain from the stack file.
		importTrees := make(map[string][]*tree.ImportNode)
		for stackName, componentImports := range importTreesWithComponents {
			// Filter by component if specified.
			if opts.Component != "" {
				// Only include this stack if it has the requested component.
				if _, hasComponent := componentImports[opts.Component]; !hasComponent {
					continue
				}
			}

			// Just take the first component's imports (they're all the same for a stack file).
			for _, imports := range componentImports {
				importTrees[stackName] = imports
				break
			}
		}

		// Render the tree.
		// Use showImports from --provenance flag.
		output := format.RenderStacksTree(importTrees, opts.Provenance)
		fmt.Println(output)
		return nil
	}

	// Build filters.
	filters := buildStackFilters(opts)

	// Get column configuration.
	columns := getStackColumns(&atmosConfig, opts.Columns, opts.Component != "")

	// Build column selector.
	selector, err := column.NewSelector(columns, column.BuildColumnFuncMap())
	if err != nil {
		return fmt.Errorf("error creating column selector: %w", err)
	}

	// Build sorters.
	sorters, err := buildStackSorters(opts.Sort)
	if err != nil {
		return fmt.Errorf("error parsing sort specification: %w", err)
	}

	// Create renderer and execute pipeline.
	outputFormat := format.Format(opts.Format)
	r := renderer.New(filters, selector, sorters, outputFormat)

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

// buildStackSorters creates sorters from sort specification.
func buildStackSorters(sortSpec string) ([]*listSort.Sorter, error) {
	if sortSpec == "" {
		// Default sort: by stack ascending.
		return []*listSort.Sorter{
			listSort.NewSorter("Stack", listSort.Ascending),
		}, nil
	}

	return listSort.ParseSortSpec(sortSpec)
}
