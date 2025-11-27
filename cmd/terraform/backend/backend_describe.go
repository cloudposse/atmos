package backend

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

var describeParser *flags.StandardParser

var describeCmd = &cobra.Command{
	Use:   "describe <component>",
	Short: "Describe backend configuration",
	Long: `Show component's backend configuration from stack.

Returns the actual stack configuration for the backend, not a schema.
This includes backend settings, variables, and metadata from the stack manifest.`,
	Example: `  atmos terraform backend describe vpc --stack dev
  atmos terraform backend describe vpc --stack dev --format json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "backend.describe.RunE")()

		component := args[0]

		// Parse flags using StandardParser with Viper precedence.
		v := viper.GetViper()
		if err := describeParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts, err := ParseCommonFlags(cmd, describeParser)
		if err != nil {
			return err
		}

		format := v.GetString("format")

		// Initialize config using injected dependency.
		atmosConfig, _, err := configInit.InitConfigAndAuth(component, opts.Stack, opts.Identity)
		if err != nil {
			return err
		}

		// Execute describe command using injected provisioner.
		// Pass format in a simple map since opts interface{} accepts anything.
		return prov.DescribeBackend(atmosConfig, component, map[string]string{"format": format})
	},
}

func init() {
	describeCmd.DisableFlagParsing = false

	// Create parser with functional options.
	describeParser = flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithIdentityFlag(),
		flags.WithStringFlag("format", "f", "yaml", "Output format: yaml, json, table"),
		flags.WithEnvVars("format", "ATMOS_FORMAT"),
	)

	// Register flags with the command.
	describeParser.RegisterFlags(describeCmd)

	// Bind flags to Viper.
	if err := describeParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
