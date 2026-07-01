package backend

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/terraform/shared"
	"github.com/cloudposse/atmos/pkg/flags"
)

var createParser *flags.StandardParser

var createCmd = &cobra.Command{
	Use:     "create [component]",
	Short:   "Provision backend infrastructure",
	Long:    `Create or update S3 backend with secure defaults (versioning, encryption, public access blocking). This operation is idempotent.`,
	Example: `  atmos terraform backend create vpc --stack dev`,
	// Args validator is auto-set by parser via SetPositionalArgs with prompt-aware validation.
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		v := viper.GetViper()
		if err := createParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}
		stack := getCommandFlagString(cmd, "stack")
		identity := getCommandFlagString(cmd, "identity")
		if stack == "" {
			stack = v.GetString("stack")
		}
		if identity == "" {
			identity = v.GetString("identity")
		}
		result, err := createParser.Parse(ctx, args)
		if err != nil {
			return err
		}
		if stack == "" {
			stack = result.Stack
		}
		if identity == "" {
			identity = result.Identity.Value()
		}

		return executeProvisionCommandWithValues(result.Component, stack, identity)
	},
}

func init() {
	createCmd.DisableFlagParsing = false

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
	createParser = flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithIdentityFlag(),
		// Enable prompting for missing stack flag.
		flags.WithCompletionPrompt("stack", "Choose a stack", shared.StackFlagCompletion),
		// Enable prompting for missing component positional arg.
		flags.WithPositionalArgPrompt("component", "Choose a component", shared.ComponentsArgCompletion),
	)

	// Configure positional args (auto-sets prompt-aware validator).
	createParser.SetPositionalArgs(specs, nil, usage)

	createParser.RegisterFlags(createCmd)

	if err := createParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
