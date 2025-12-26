package list

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/list/format"
)

var instancesParser *flags.StandardParser

// InstancesOptions contains parsed flags for the instances command.
type InstancesOptions struct {
	global.Flags
	Format     string
	Columns    []string
	MaxColumns int
	Delimiter  string
	Stack      string
	Filter     string
	Query      string
	Sort       string
	Upload     bool
	Provenance bool
}

// instancesCmd lists atmos instances.
var instancesCmd = &cobra.Command{
	Use:                "instances",
	Short:              "List all Atmos instances",
	Long:               "This command lists all Atmos instances or is used to upload instances to the pro API.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get Viper instance for flag/env precedence.
		v := viper.GetViper()

		// Check Atmos configuration (honors --base-path, --config, --config-path, --profile).
		if err := checkAtmosConfig(cmd, v); err != nil {
			return err
		}

		// Parse flags using StandardParser with Viper precedence.
		if err := instancesParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := &InstancesOptions{
			Flags:      flags.ParseGlobalFlags(cmd, v),
			Format:     v.GetString("format"),
			Columns:    v.GetStringSlice("columns"),
			MaxColumns: v.GetInt("max-columns"),
			Delimiter:  v.GetString("delimiter"),
			Stack:      v.GetString("stack"),
			Filter:     v.GetString("filter"),
			Query:      v.GetString("query"),
			Sort:       v.GetString("sort"),
			Upload:     v.GetBool("upload"),
			Provenance: v.GetBool("provenance"),
		}

		return executeListInstancesCmd(cmd, args, opts)
	},
}

// columnsCompletionForInstances provides dynamic tab completion for --columns flag.
// Returns column names from atmos.yaml components.list.columns configuration.
func columnsCompletionForInstances(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Load atmos configuration.
	configAndStacksInfo, err := e.ProcessCommandLineArgs("list", cmd, args, nil)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, false)
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
	// Create parser using flag wrappers.
	instancesParser = NewListParser(
		WithFormatFlag,
		WithInstancesColumnsFlag,
		WithDelimiterFlag,
		WithMaxColumnsFlag,
		WithStackFlag,
		WithFilterFlag,
		WithQueryFlag,
		WithSortFlag,
		WithUploadFlag,
		WithProvenanceFlag,
	)

	// Register flags.
	instancesParser.RegisterFlags(instancesCmd)

	// Register dynamic tab completion for --columns flag.
	if err := instancesCmd.RegisterFlagCompletionFunc("columns", columnsCompletionForInstances); err != nil {
		panic(err)
	}

	// Bind flags to Viper for environment variable support.
	if err := instancesParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func executeListInstancesCmd(cmd *cobra.Command, args []string, opts *InstancesOptions) error {
	// Validate that --provenance only works with --format=tree.
	if opts.Provenance && opts.Format != string(format.FormatTree) {
		return fmt.Errorf("%w: --provenance flag only works with --format=tree", errUtils.ErrInvalidFlag)
	}

	// Process and validate command line arguments.
	configAndStacksInfo, err := e.ProcessCommandLineArgs("list", cmd, args, nil)
	if err != nil {
		return err
	}
	configAndStacksInfo.Command = "list"
	configAndStacksInfo.SubCommand = "instances"

	// Initialize config to create auth manager.
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return err
	}

	// Create AuthManager for authentication support.
	authManager, err := createAuthManagerForList(cmd, &atmosConfig)
	if err != nil {
		return err
	}

	return list.ExecuteListInstancesCmd(&list.InstancesCommandOptions{
		Info:        &configAndStacksInfo,
		Cmd:         cmd,
		Args:        args,
		ShowImports: opts.Provenance,
		ColumnsFlag: opts.Columns,
		FilterSpec:  opts.Filter,
		SortSpec:    opts.Sort,
		Delimiter:   opts.Delimiter,
		Query:       opts.Query,
		AuthManager: authManager,
	})
}
