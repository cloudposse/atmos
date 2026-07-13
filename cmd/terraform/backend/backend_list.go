package backend

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/terraform/shared"
	"github.com/cloudposse/atmos/pkg/flags"
)

var listParser *flags.StandardParser

var listCmd = &cobra.Command{
	Use:     "list",
	Short:   "List all backends in stack",
	Long:    `Show all provisioned backends and their status for a given stack.`,
	Example: `  atmos terraform backend list --stack dev`,
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		v := viper.GetViper()
		if err := listParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}
		if _, err := listParser.Parse(ctx, args); err != nil {
			return err
		}

		// Prefer the CLI-supplied value (getCommandFlagString) over Viper: Viper's precedence
		// gives an explicit Set() call priority over a bound pflag, so a caller that pre-seeds
		// Viper (e.g. from config defaults) can otherwise shadow the flag the user just passed.
		stack := getCommandFlagString(cmd, "stack")
		if stack == "" {
			stack = v.GetString("stack")
		}
		identity := getCommandFlagString(cmd, "identity")
		if identity == "" {
			identity = v.GetString("identity")
		}
		format := getCommandFlagString(cmd, "format")
		if format == "" {
			format = v.GetString("format")
		}

		return executeListCommandWithValues(stack, identity, format)
	},
}

func init() {
	listCmd.DisableFlagParsing = false

	// Create parser with prompting options.
	listParser = flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithIdentityFlag(),
		flags.WithStringFlag("format", "f", "table", "Output format: table, yaml, json"),
		flags.WithEnvVars("format", "ATMOS_FORMAT"),
		// Enable prompting for missing stack flag.
		flags.WithCompletionPrompt("stack", "Choose a stack", shared.StackFlagCompletion),
	)

	// Register flags with the command.
	listParser.RegisterFlags(listCmd)

	// Bind flags to Viper.
	if err := listParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
