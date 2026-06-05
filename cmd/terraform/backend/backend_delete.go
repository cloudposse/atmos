package backend

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/terraform/shared"
	"github.com/cloudposse/atmos/pkg/flags"
)

var deleteParser *flags.StandardParser

var deleteCmd = &cobra.Command{
	Use:   "delete [component]",
	Short: "Delete backend infrastructure",
	Long: `Permanently delete backend infrastructure.

Requires the --force flag for safety. The backend must be empty
(no state files) before it can be deleted.`,
	Example: `  atmos terraform backend delete vpc --stack dev --force`,
	// Args validator is auto-set by parser via SetPositionalArgs with prompt-aware validation.
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		result, err := deleteParser.Parse(ctx, args)
		if err != nil {
			return err
		}

		// Get force flag from Viper (not a standard StandardOptions field).
		v := viper.GetViper()
		force := v.GetBool("force")

		return executeDeleteCommandWithValues(result.Component, result.Stack, result.Identity.Value(), force)
	},
}

func init() {
	deleteCmd.DisableFlagParsing = false

	// Build positional args with prompting.
	argsBuilder := flags.NewPositionalArgsBuilder()
	argsBuilder.AddArg(&flags.PositionalArgSpec{
		Name:           "component",
		Description:    "Component name",
		Required:       true,
		TargetField:    "Component",
		CompletionFunc: shared.ComponentsArgCompletion,
		PromptTitle:    "Choose a component",
	})
	specs, _, usage := argsBuilder.Build()

	// Create parser with prompting options.
	deleteParser = flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithIdentityFlag(),
		flags.WithBoolFlag("force", "", false, "Force deletion without confirmation"),
		flags.WithEnvVars("force", "ATMOS_FORCE"),
		// Enable prompting for missing stack flag.
		flags.WithCompletionPrompt("stack", "Choose a stack", shared.StackFlagCompletion),
		// Enable prompting for missing component positional arg.
		flags.WithPositionalArgPrompt("component", "Choose a component", shared.ComponentsArgCompletion),
	)

	// Configure positional args (auto-sets prompt-aware validator).
	deleteParser.SetPositionalArgs(specs, nil, usage)

	// Register flags with the command.
	deleteParser.RegisterFlags(deleteCmd)

	// Bind flags to Viper.
	if err := deleteParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
