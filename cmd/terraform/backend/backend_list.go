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
		result, err := listParser.Parse(ctx, args)
		if err != nil {
			return err
		}

		stack := result.Stack
		if stack == "" {
			stack = getCommandFlagStack(cmd)
		}
		if stack == "" {
			stack = v.GetString("stack")
		}
		identity := flags.ParseGlobalFlags(cmd, v).Identity.Value()
		return executeListCommandWithValues(stack, identity, result.Format)
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
