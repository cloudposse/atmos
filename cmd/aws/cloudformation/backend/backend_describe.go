package backend

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
)

var describeParser *flags.StandardParser

var describeCmd = &cobra.Command{
	Use:   "describe [component]",
	Short: "Describe the template-packaging backend bucket",
	Long: `Show whether the S3 bucket referenced by the component's "kind: aws/s3"
provision target currently exists, along with its resolved bucket/region.`,
	Example: `  atmos aws cloudformation backend describe vpc --stack dev
  atmos aws cloudformation backend describe vpc --stack dev --format json`,
	// Args validator is auto-set by parser via SetPositionalArgs with prompt-aware validation.
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		v := viper.GetViper()
		if err := describeParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}
		result, err := describeParser.Parse(ctx, args)
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
		return executeDescribe(ctx, &describeRequest{
			Component: result.Component,
			Stack:     stack,
			Identity:  identity,
			Target:    target,
			Format:    result.Format,
		})
	},
}

func init() {
	describeCmd.DisableFlagParsing = false

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

	describeParser = flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithIdentityFlag(),
		flags.WithStringFlag("target", "", "", "The `kind: aws/s3` provision target to use. Required when more than one is declared."),
		flags.WithStringFlag("format", "f", "table", "Output format: table, yaml, json"),
		flags.WithEnvVars("format", "ATMOS_FORMAT"),
		flags.WithCompletionPrompt("stack", "Choose a stack", stackFlagCompletion),
		flags.WithPositionalArgPrompt("component", "Choose an aws/cloudformation component", componentArgCompletion),
	)

	describeParser.SetPositionalArgs(specs, nil, usage)
	describeParser.RegisterFlags(describeCmd)

	if err := describeParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

// describeRequest bundles executeDescribe's parameters below revive's
// argument-limit (5): component/stack/identity/target/format is 6 as separate
// parameters.
type describeRequest struct {
	Component string
	Stack     string
	Identity  string
	Target    string
	Format    string
}

func executeDescribe(ctx context.Context, req *describeRequest) error {
	if req.Stack == "" {
		return errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
			WithExplanation("--stack flag is required").
			WithHint("Specify a stack with --stack or -s flag").
			Err()
	}

	atmosConfig, info, err := configInit.InitConfigAndAuth(req.Component, req.Stack, req.Identity)
	if err != nil {
		return err
	}

	componentConfig, err := configInit.DescribeComponent(atmosConfig, info, req.Component, req.Stack)
	if err != nil {
		return err
	}

	return prov.DescribeBackend(ctx, &DescribeBackendParams{
		CreateBackendParams: CreateBackendParams{
			AtmosConfig:     atmosConfig,
			Component:       req.Component,
			Stack:           req.Stack,
			ComponentConfig: componentConfig,
			AuthContext:     info.AuthContext,
			Target:          req.Target,
		},
		Format: req.Format,
	})
}
