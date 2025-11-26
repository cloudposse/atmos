package backend

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner"
)

var listParser *flags.StandardParser

var listCmd = &cobra.Command{
	Use:     "list",
	Short:   "List all backends in stack",
	Long:    `Show all provisioned backends and their status for a given stack.`,
	Example: `  atmos terraform backend list --stack dev`,
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "backend.list.RunE")()

		// Parse flags.
		v := viper.GetViper()
		if err := listParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts, err := ParseCommonFlags(cmd, listParser)
		if err != nil {
			return err
		}

		format := v.GetString("format")

		// Initialize config (no component needed for list).
		atmosConfig, _, err := InitConfigAndAuth("", opts.Stack, opts.Identity)
		if err != nil {
			return err
		}

		// Execute list command using pkg/provisioner.
		return provisioner.ListBackends(atmosConfig, map[string]string{"format": format})
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
