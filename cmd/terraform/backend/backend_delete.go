package backend

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provision"
)

var deleteParser *flags.StandardParser

var deleteCmd = &cobra.Command{
	Use:   "delete <component>",
	Short: "Delete backend infrastructure",
	Long: `Permanently delete backend infrastructure.

Requires the --force flag for safety. The backend must be empty
(no state files) before it can be deleted.`,
	Example: `  atmos terraform backend delete vpc --stack dev --force`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "backend.delete.RunE")()

		component := args[0]

		// Parse flags.
		v := viper.GetViper()
		if err := deleteParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts, err := ParseCommonFlags(cmd, deleteParser)
		if err != nil {
			return err
		}

		force := v.GetBool("force")

		// Initialize config.
		atmosConfig, _, err := InitConfigAndAuth(component, opts.Stack, opts.Identity)
		if err != nil {
			return err
		}

		// Execute delete command using pkg/provision.
		// Pass force flag in a simple map.
		return provision.DeleteBackend(atmosConfig, component, map[string]bool{"force": force})
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
