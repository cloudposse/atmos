package list

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/schema"
)

var instancesParser *flags.StandardParser

// InstancesOptions contains parsed flags for the instances command.
type InstancesOptions struct {
	global.Flags
	Format     string
	MaxColumns int
	Delimiter  string
	Stack      string
	Query      string
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
			Query:      v.GetString("query"),
			Upload:     v.GetBool("upload"),
		}

		return executeListInstancesCmd(cmd, args, opts)
	},
}

func init() {
	// Create parser with common list flags plus upload flag
	instancesParser = newCommonListParser(
		flags.WithBoolFlag("upload", "", false, "Upload instances to pro API"),
		flags.WithEnvVars("upload", "ATMOS_LIST_UPLOAD"),
	)

	// Register flags
	instancesParser.RegisterFlags(instancesCmd)

	// Bind flags to Viper for environment variable support
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

	// Load atmos configuration to get auth config.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}

	// Get identity from flag (if --identity flag is added to this command).
	identityName := ""
	if cmd.Flags().Changed("identity") {
		identityName, _ = cmd.Flags().GetString("identity")
	}

	// Create AuthManager with stack-level default identity scanning.
	// This enables stack-level auth.identities.*.default to be recognized.
	authManager, err := auth.CreateAndAuthenticateManagerWithAtmosConfig(
		identityName,
		&atmosConfig.Auth,
		cfg.IdentityFlagSelectValue,
		&atmosConfig,
	)
	if err != nil {
		return err
	}

	return list.ExecuteListInstancesCmd(&configAndStacksInfo, cmd, args, authManager)
}
