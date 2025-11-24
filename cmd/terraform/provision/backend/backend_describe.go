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

var describeParser *flags.StandardParser

// DescribeOptions contains parsed flags for the describe command.
type DescribeOptions struct {
	global.Flags
	Stack    string
	Identity string
	Format   string
}

var describeCmd = &cobra.Command{
	Use:   "describe <component>",
	Short: "Describe backend configuration",
	Long: `Show component's backend configuration from stack.

Returns the actual stack configuration for the backend, not a schema.
This includes backend settings, variables, and metadata from the stack manifest.`,
	Example: `  atmos terraform provision backend describe vpc --stack dev
  atmos terraform provision backend describe vpc --stack dev --format json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "backend.describe.RunE")()

		component := args[0]

		// Parse flags using StandardParser with Viper precedence.
		v := viper.GetViper()
		if err := describeParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := &DescribeOptions{
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
			ComponentFromArg: component,
			Stack:            opts.Stack,
		}, false)
		if err != nil {
			return errors.Join(errUtils.ErrFailedToInitConfig, err)
		}

		// Execute describe command using pkg/provision.
		return provision.DescribeBackend(&atmosConfig, component, opts)
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
