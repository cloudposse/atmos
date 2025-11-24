package backend

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provision"
	"github.com/cloudposse/atmos/pkg/schema"
)

var listParser *flags.StandardParser

// ListOptions contains parsed flags for the list command.
type ListOptions struct {
	global.Flags
	Stack    string
	Identity string
	Format   string
}

var listCmd = &cobra.Command{
	Use:     "list",
	Short:   "List all backends in stack",
	Long:    `Show all provisioned backends and their status for a given stack.`,
	Example: `  atmos terraform provision backend list --stack dev`,
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "backend.list.RunE")()

		// Parse flags using StandardParser with Viper precedence.
		v := viper.GetViper()
		if err := listParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := &ListOptions{
			Flags:    flags.ParseGlobalFlags(cmd, v),
			Stack:    v.GetString("stack"),
			Identity: v.GetString("identity"),
			Format:   v.GetString("format"),
		}

		if opts.Stack == "" {
			return errUtils.ErrRequiredFlagNotProvided
		}

		// Load atmos configuration.
		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{
			Stack: opts.Stack,
		}, false)
		if err != nil {
			return errors.Join(errUtils.ErrFailedToInitConfig, err)
		}

		// Execute list command using pkg/provision.
		return provision.ListBackends(&atmosConfig, opts)
	},
}

func init() {
	listCmd.DisableFlagParsing = false

	// Create parser with functional options.
	listParser = flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithIdentityFlag(),
		flags.WithStringFlag("format", "f", "table", "Output format: table, yaml, json"),
		flags.WithEnvVars("format", "ATMOS_FORMAT"),
	)

	// Register flags with the command.
	listParser.RegisterFlags(listCmd)

	// Bind flags to Viper.
	if err := listParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
