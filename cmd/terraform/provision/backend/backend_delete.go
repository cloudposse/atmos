//nolint:dupl // CRUD commands share similar structure intentionally - standard command pattern.
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

var deleteParser *flags.StandardParser

// DeleteOptions contains parsed flags for the delete command.
type DeleteOptions struct {
	global.Flags
	Stack    string
	Identity string
	Force    bool
}

var deleteCmd = &cobra.Command{
	Use:   "delete <component>",
	Short: "Delete backend infrastructure",
	Long: `Permanently delete backend infrastructure.

Requires the --force flag for safety. The backend must be empty
(no state files) before it can be deleted.`,
	Example: `  atmos terraform provision backend delete vpc --stack dev --force`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "backend.delete.RunE")()

		component := args[0]

		// Parse flags using StandardParser with Viper precedence.
		v := viper.GetViper()
		if err := deleteParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := &DeleteOptions{
			Flags:    flags.ParseGlobalFlags(cmd, v),
			Stack:    v.GetString("stack"),
			Identity: v.GetString("identity"),
			Force:    v.GetBool("force"),
		}

		if opts.Stack == "" {
			return errUtils.ErrRequiredFlagNotProvided
		}

		// Load atmos configuration.
		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{
			ComponentFromArg: component,
			Stack:            opts.Stack,
		}, false)
		if err != nil {
			return errors.Join(errUtils.ErrFailedToInitConfig, err)
		}

		// Execute delete command using pkg/provision.
		return provision.DeleteBackend(&atmosConfig, component, opts)
	},
}

func init() {
	deleteCmd.DisableFlagParsing = false

	// Create parser with functional options.
	deleteParser = flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithIdentityFlag(),
		flags.WithBoolFlag("force", "", false, "Force deletion without confirmation"),
		flags.WithEnvVars("force", "ATMOS_FORCE"),
	)

	// Register flags with the command.
	deleteParser.RegisterFlags(deleteCmd)

	// Bind flags to Viper.
	if err := deleteParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
