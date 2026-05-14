package backend

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/terraform/shared"
	"github.com/cloudposse/atmos/pkg/flags"
)

var updateParser *flags.StandardParser

var updateCmd = &cobra.Command{
	Use:   "update [component]",
	Short: "Update backend configuration",
	Long: `Apply configuration changes to existing backend.

This operation is idempotent and will update backend settings like
versioning, encryption, and public access blocking to match secure defaults.`,
	Example: `  atmos terraform backend update vpc --stack dev`,
	// Args validator is auto-set by parser via SetPositionalArgs with prompt-aware validation.
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		result, err := updateParser.Parse(ctx, args)
		if err != nil {
			return err
		}

		return executeProvisionCommandWithValues(result.Component, result.Stack, result.Identity.Value())
	},
}

func init() {
	updateCmd.DisableFlagParsing = false

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
	updateParser = flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithIdentityFlag(),
		// Enable prompting for missing stack flag.
		flags.WithCompletionPrompt("stack", "Choose a stack", shared.StackFlagCompletion),
		// Enable prompting for missing component positional arg.
		flags.WithPositionalArgPrompt("component", "Choose a component", shared.ComponentsArgCompletion),
	)

	// Configure positional args (auto-sets prompt-aware validator).
	updateParser.SetPositionalArgs(specs, nil, usage)

	updateParser.RegisterFlags(updateCmd)

	if err := updateParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
