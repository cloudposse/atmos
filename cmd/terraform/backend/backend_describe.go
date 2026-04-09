package backend

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/terraform/shared"
	"github.com/cloudposse/atmos/pkg/flags"
)

var describeParser *flags.StandardParser

var describeCmd = &cobra.Command{
	Use:   "describe [component]",
	Short: "Describe backend configuration",
	Long: `Show component's backend configuration from stack.

Returns the actual stack configuration for the backend, not a schema.
This includes backend settings, variables, and metadata from the stack manifest.`,
	Example: `  atmos terraform backend describe vpc --stack dev
  atmos terraform backend describe vpc --stack dev --format json`,
	// Args validator is auto-set by parser via SetPositionalArgs with prompt-aware validation.
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		result, err := describeParser.Parse(ctx, args)
		if err != nil {
			return err
		}

		return executeDescribeCommandWithValues(result.Component, result.Stack, result.Identity.Value(), result.Format)
	},
}

func init() {
	describeCmd.DisableFlagParsing = false

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
	describeParser = flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithIdentityFlag(),
		flags.WithStringFlag("format", "f", "yaml", "Output format: yaml, json, table"),
		flags.WithEnvVars("format", "ATMOS_FORMAT"),
		// Enable prompting for missing stack flag.
		flags.WithCompletionPrompt("stack", "Choose a stack", shared.StackFlagCompletion),
		// Enable prompting for missing component positional arg.
		flags.WithPositionalArgPrompt("component", "Choose a component", shared.ComponentsArgCompletion),
	)

	// Configure positional args (auto-sets prompt-aware validator).
	describeParser.SetPositionalArgs(specs, nil, usage)

	// Register flags with the command.
	describeParser.RegisterFlags(describeCmd)

	// Bind flags to Viper.
	if err := describeParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
