package list

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

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
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

var stacksParser *flags.StandardParser

// StacksOptions contains parsed flags for the stacks command.
type StacksOptions struct {
	global.Flags
	Component string
	Format    string
	Columns   string
	Sort      string
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
			Flags:     flags.ParseGlobalFlags(cmd, v),
			Component: v.GetString("component"),
			Format:    v.GetString("format"),
			Columns:   v.GetString("columns"),
			Sort:      v.GetString("sort"),
		}

		return listStacksWithOptions(opts)
	},
}

func init() {
	// Create parser with stacks-specific flags using flag wrappers.
	stacksParser = NewListParser(
		WithFormatFlag,
		WithColumnsFlag,
		WithSortFlag,
		WithComponentFlag,
	)

	// Register flags.
	stacksParser.RegisterFlags(stacksCmd)

	// Bind flags to Viper for environment variable support.
	if err := stacksParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func listStacksWithOptions(opts *StacksOptions) error {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return fmt.Errorf("error initializing CLI config: %v", err)
	}

	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return fmt.Errorf("error describing stacks: %v", err)
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
		ui.Info("No stacks found")
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
func getStackColumns(atmosConfig *schema.AtmosConfiguration, columnsFlag string, hasComponent bool) []column.Config {
	// If --columns flag is provided, parse it and return.
	if columnsFlag != "" {
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
