package list

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/filter"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	listSort "github.com/cloudposse/atmos/pkg/list/sort"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

var workflowsParser *flags.StandardParser

// WorkflowsOptions contains parsed flags for the workflows command.
type WorkflowsOptions struct {
	global.Flags
	File    string
	Format  string
	Columns []string
	Sort    string
}

// workflowsCmd lists atmos workflows.
var workflowsCmd = &cobra.Command{
	Use:   "workflows",
	Short: "List all Atmos workflows with filtering, sorting, and formatting options",
	Long:  `List Atmos workflows with support for filtering by file, custom column selection, sorting, and multiple output formats.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Skip stack validation for workflows.
		if err := checkAtmosConfig(true); err != nil {
			return err
		}

		// Parse flags using StandardParser with Viper precedence.
		v := viper.GetViper()
		if err := workflowsParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := &WorkflowsOptions{
			Flags:   flags.ParseGlobalFlags(cmd, v),
			File:    v.GetString("file"),
			Format:  v.GetString("format"),
			Columns: v.GetStringSlice("columns"),
			Sort:    v.GetString("sort"),
		}

		return listWorkflowsWithOptions(opts)
	},
}

func init() {
	// Create parser with workflows-specific flags using flag wrappers.
	workflowsParser = NewListParser(
		WithFormatFlag,
		WithColumnsFlag,
		WithSortFlag,
		WithFileFlag,
	)

	// Register flags.
	workflowsParser.RegisterFlags(workflowsCmd)

	// Bind flags to Viper for environment variable support.
	if err := workflowsParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func listWorkflowsWithOptions(opts *WorkflowsOptions) error {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, false)
	if err != nil {
		return err
	}

	// If format is empty, check command-specific config.
	if opts.Format == "" && atmosConfig.Workflows.List.Format != "" {
		opts.Format = atmosConfig.Workflows.List.Format
	}

	// Extract workflows into structured data.
	workflows, err := l.ExtractWorkflows(&atmosConfig, opts.File)
	if err != nil {
		return err
	}

	if len(workflows) == 0 {
		_ = ui.Info("No workflows found")
		return nil
	}

	// Build filters.
	filters := buildWorkflowFilters(opts)

	// Get column configuration.
	columns := getWorkflowColumns(&atmosConfig, opts.Columns)

	// Build column selector.
	selector, err := column.NewSelector(columns, column.BuildColumnFuncMap())
	if err != nil {
		return fmt.Errorf("error creating column selector: %w", err)
	}

	// Build sorters.
	sorters, err := buildWorkflowSorters(opts.Sort)
	if err != nil {
		return fmt.Errorf("error parsing sort specification: %w", err)
	}

	// Create renderer and execute pipeline.
	outputFormat := format.Format(opts.Format)
	r := renderer.New(filters, selector, sorters, outputFormat)

	return r.Render(workflows)
}

// buildWorkflowFilters creates filters based on command options.
func buildWorkflowFilters(opts *WorkflowsOptions) []filter.Filter {
	var filters []filter.Filter

	// File filter already handled by extraction logic.
	// Add any additional filters here in the future.

	return filters
}

// getWorkflowColumns returns column configuration.
func getWorkflowColumns(atmosConfig *schema.AtmosConfiguration, columnsFlag []string) []column.Config {
	// If --columns flag is provided, parse it and return.
	if len(columnsFlag) > 0 {
		return parseColumnsFlag(columnsFlag)
	}

	// Check atmos.yaml for workflows.list.columns configuration.
	if len(atmosConfig.Workflows.List.Columns) > 0 {
		var configs []column.Config
		for _, col := range atmosConfig.Workflows.List.Columns {
			configs = append(configs, column.Config{
				Name:  col.Name,
				Value: col.Value,
			})
		}
		return configs
	}

	// Default columns for workflows.
	return []column.Config{
		{Name: "File", Value: "{{ .file }}"},
		{Name: "Workflow", Value: "{{ .name }}"},
		{Name: "Description", Value: "{{ .description }}"},
		{Name: "Steps", Value: "{{ .steps }}"},
	}
}

// buildWorkflowSorters creates sorters from sort specification.
func buildWorkflowSorters(sortSpec string) ([]*listSort.Sorter, error) {
	if sortSpec == "" {
		// Default sort: by file ascending, then workflow ascending.
		return []*listSort.Sorter{
			listSort.NewSorter("File", listSort.Ascending),
			listSort.NewSorter("Workflow", listSort.Ascending),
		}, nil
	}

	return listSort.ParseSortSpec(sortSpec)
}
