package list

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/list"
)

var metadataParser *flags.StandardParser

// MetadataOptions contains parsed flags for the metadata command.
type MetadataOptions struct {
	global.Flags
	Format  string
	Stack   string
	Columns []string
	Sort    string
	Filter  string
}

// metadataCmd lists metadata across stacks.
var metadataCmd = &cobra.Command{
	Use:   "metadata",
	Short: "List metadata across stacks",
	Long:  "List metadata information across all stacks with customizable columns",
	Example: "atmos list metadata\n" +
		"atmos list metadata --format json\n" +
		"atmos list metadata --stack 'plat-*-prod'\n" +
		"atmos list metadata --columns stack,component,type,enabled\n" +
		"atmos list metadata --sort stack:asc,component:desc\n" +
		"atmos list metadata --filter '.enabled == true'",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get Viper instance for flag/env precedence.
		v := viper.GetViper()

		// Check Atmos configuration (honors --base-path, --config, --config-path, --profile).
		if err := checkAtmosConfig(cmd, v); err != nil {
			return err
		}

		// Parse flags using StandardParser with Viper precedence.
		if err := metadataParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := &MetadataOptions{
			Flags:   flags.ParseGlobalFlags(cmd, v),
			Format:  v.GetString("format"),
			Stack:   v.GetString("stack"),
			Columns: v.GetStringSlice("columns"),
			Sort:    v.GetString("sort"),
			Filter:  v.GetString("filter"),
		}

		return executeListMetadataCmd(cmd, args, opts)
	},
}

// columnsCompletionForMetadata provides dynamic tab completion for --columns flag.
// Returns column names from atmos.yaml components.list.columns configuration.
func columnsCompletionForMetadata(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
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
	metadataParser = NewListParser(
		WithFormatFlag,
		WithStackFlag,
		WithMetadataColumnsFlag,
		WithSortFlag,
		WithFilterFlag,
	)

	// Register flags.
	metadataParser.RegisterFlags(metadataCmd)

	// Register dynamic tab completion for --columns flag.
	if err := metadataCmd.RegisterFlagCompletionFunc("columns", columnsCompletionForMetadata); err != nil {
		panic(err)
	}

	// Bind flags to Viper for environment variable support.
	if err := metadataParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func executeListMetadataCmd(cmd *cobra.Command, args []string, opts *MetadataOptions) error {
	// Process and validate command line arguments.
	configAndStacksInfo, err := e.ProcessCommandLineArgs("list", cmd, args, nil)
	if err != nil {
		return err
	}
	configAndStacksInfo.Command = "list"
	configAndStacksInfo.SubCommand = "metadata"

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

	// Convert cmd-level options to pkg-level options.
	pkgOpts := &list.MetadataOptions{
		Format:      opts.Format,
		Columns:     opts.Columns,
		Sort:        opts.Sort,
		Filter:      opts.Filter,
		Stack:       opts.Stack,
		AuthManager: authManager,
	}

	return list.ExecuteListMetadataCmd(&configAndStacksInfo, cmd, args, pkgOpts)
}
