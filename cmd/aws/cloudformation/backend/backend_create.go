package backend

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
)

var createParser *flags.StandardParser

var createCmd = &cobra.Command{
	Use:     "create [component]",
	Short:   "Provision the template-packaging backend bucket",
	Long:    `Create or update the S3 bucket referenced by the component's "kind: aws/s3" provision target, with secure defaults (versioning, encryption, public access blocking). This operation is idempotent.`,
	Example: `  atmos aws cloudformation backend create vpc --stack dev`,
	// Args validator is auto-set by parser via SetPositionalArgs with prompt-aware validation.
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		v := viper.GetViper()
		if err := createParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}
		result, err := createParser.Parse(ctx, args)
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
		target := v.GetString("target")
		return executeCreateOrUpdate(ctx, result.Component, stack, identity, target)
	},
}

func init() {
	createCmd.DisableFlagParsing = false

	argsBuilder := flags.NewPositionalArgsBuilder()
	argsBuilder.AddArg(&flags.PositionalArgSpec{
		Name:           "component",
		Description:    "aws/cloudformation component",
		Required:       true,
		TargetField:    "Component",
		CompletionFunc: componentArgCompletion,
		PromptTitle:    "Choose an aws/cloudformation component",
	})
	specs, _, usage := argsBuilder.Build()

	createParser = flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithIdentityFlag(),
		flags.WithStringFlag("target", "", "", "The `kind: aws/s3` provision target to use. Required when more than one is declared."),
		flags.WithCompletionPrompt("stack", "Choose a stack", stackFlagCompletion),
		flags.WithPositionalArgPrompt("component", "Choose an aws/cloudformation component", componentArgCompletion),
	)

	createParser.SetPositionalArgs(specs, nil, usage)
	createParser.RegisterFlags(createCmd)

	if err := createParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

// executeCreateOrUpdate is shared by `create` and `update`: both provision the
// backend via the same idempotent ProvisionWithParams code path.
func executeCreateOrUpdate(ctx context.Context, component, stack, identity, target string) error {
	if stack == "" {
		return errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
			WithExplanation("--stack flag is required").
			WithHint("Specify a stack with --stack or -s flag").
			Err()
	}

	atmosConfig, info, err := configInit.InitConfigAndAuth(component, stack, identity)
	if err != nil {
		return err
	}

	componentConfig, err := configInit.DescribeComponent(atmosConfig, info, component, stack)
	if err != nil {
		return err
	}

	return prov.CreateBackend(ctx, &CreateBackendParams{
		AtmosConfig:     atmosConfig,
		Component:       component,
		Stack:           stack,
		ComponentConfig: componentConfig,
		AuthContext:     info.AuthContext,
		Target:          target,
	})
}
