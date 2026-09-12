package backend

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
)

var listParser *flags.StandardParser

var listCmd = &cobra.Command{
	Use:   "list [component]",
	Short: "List the component's template-packaging backend targets",
	Long: `List every "kind: aws/s3" provision target declared on the component, and
whether each one's bucket currently exists.

Unlike "atmos terraform backend list" (which lists every backend across a
whole stack), this is scoped to one component: CFN's provision-target model
declares "kind: aws/s3" targets per-component, not once per stack.`,
	Example: `  atmos aws cloudformation backend list vpc --stack dev`,
	// Args validator is auto-set by parser via SetPositionalArgs with prompt-aware validation.
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
		return executeList(ctx, result.Component, stack, identity, result.Format)
	},
}

func init() {
	listCmd.DisableFlagParsing = false

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

	listParser = flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithIdentityFlag(),
		flags.WithStringFlag("format", "f", "table", "Output format: table, yaml, json"),
		flags.WithEnvVars("format", "ATMOS_FORMAT"),
		flags.WithCompletionPrompt("stack", "Choose a stack", stackFlagCompletion),
		flags.WithPositionalArgPrompt("component", "Choose an aws/cloudformation component", componentArgCompletion),
	)

	listParser.SetPositionalArgs(specs, nil, usage)
	listParser.RegisterFlags(listCmd)

	if err := listParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func executeList(ctx context.Context, component, stack, identity, format string) error {
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

	return prov.ListBackends(ctx, &ListBackendsParams{
		AtmosConfig:     atmosConfig,
		Component:       component,
		ComponentConfig: componentConfig,
		AuthContext:     info.AuthContext,
		Format:          format,
	})
}
