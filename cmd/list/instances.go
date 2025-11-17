package list

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/list"
)

var instancesParser *flags.StandardParser

// InstancesOptions contains parsed flags for the instances command.
type InstancesOptions struct {
	global.Flags
	Format     string
	MaxColumns int
	Delimiter  string
	Stack      string
	Filter     string
	Query      string
	Sort       string
	Upload     bool
}

// instancesCmd lists atmos instances.
var instancesCmd = &cobra.Command{
	Use:                "instances",
	Short:              "List all Atmos instances",
	Long:               "This command lists all Atmos instances or is used to upload instances to the pro API.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration
		if err := checkAtmosConfig(); err != nil {
			return err
		}

		// Parse flags using StandardParser with Viper precedence
		v := viper.GetViper()
		if err := instancesParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := &InstancesOptions{
			Flags:      flags.ParseGlobalFlags(cmd, v),
			Format:     v.GetString("format"),
			MaxColumns: v.GetInt("max-columns"),
			Delimiter:  v.GetString("delimiter"),
			Stack:      v.GetString("stack"),
			Filter:     v.GetString("filter"),
			Query:      v.GetString("query"),
			Sort:       v.GetString("sort"),
			Upload:     v.GetBool("upload"),
		}

		return executeListInstancesCmd(cmd, args, opts)
	},
}

func init() {
	// Create parser using flag wrappers.
	instancesParser = NewListParser(
		WithFormatFlag,
		WithDelimiterFlag,
		WithMaxColumnsFlag,
		WithStackFlag,
		WithFilterFlag,
		WithQueryFlag,
		WithSortFlag,
		WithUploadFlag,
	)

	// Register flags.
	instancesParser.RegisterFlags(instancesCmd)

	// Bind flags to Viper for environment variable support.
	if err := instancesParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func executeListInstancesCmd(cmd *cobra.Command, args []string, opts *InstancesOptions) error {
	// Process and validate command line arguments.
	configAndStacksInfo, err := e.ProcessCommandLineArgs("list", cmd, args, nil)
	if err != nil {
		return err
	}
	configAndStacksInfo.Command = "list"
	configAndStacksInfo.SubCommand = "instances"

	return list.ExecuteListInstancesCmd(&configAndStacksInfo, cmd, args)
}
