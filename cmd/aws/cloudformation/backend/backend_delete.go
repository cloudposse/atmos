package backend

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
)

var deleteParser *flags.StandardParser

var deleteCmd = &cobra.Command{
	Use:   "delete [component]",
	Short: "Delete the template-packaging backend bucket",
	Long: `Permanently delete the S3 bucket referenced by the component's "kind: aws/s3"
provision target, and every object it contains.

Requires the --force flag for safety. This action cannot be undone.`,
	Example: `  atmos aws cloudformation backend delete vpc --stack dev --force`,
	// Args validator is auto-set by parser via SetPositionalArgs with prompt-aware validation.
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		v := viper.GetViper()
		if err := deleteParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}
		result, err := deleteParser.Parse(ctx, args)
		if err != nil {
			return err
		}

		// Component comes from result since it may have been filled in by the interactive
		// prompt. force isn't a StandardOptions field, so it's read here too.
		force, forceProvided := getCommandFlagBool(cmd, "force")
		if !forceProvided {
			force = v.GetBool("force")
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
		return executeDelete(ctx, deleteRequest{
			Component: result.Component,
			Stack:     stack,
			Identity:  identity,
			Target:    target,
			Force:     force,
		})
	},
}

func init() {
	deleteCmd.DisableFlagParsing = false

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

	deleteParser = flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithIdentityFlag(),
		flags.WithStringFlag("target", "", "", "The `kind: aws/s3` provision target to use. Required when more than one is declared."),
		flags.WithBoolFlag("force", "", false, "Force deletion without confirmation"),
		flags.WithEnvVars("force", "ATMOS_FORCE"),
		flags.WithCompletionPrompt("stack", "Choose a stack", stackFlagCompletion),
		flags.WithPositionalArgPrompt("component", "Choose an aws/cloudformation component", componentArgCompletion),
	)

	deleteParser.SetPositionalArgs(specs, nil, usage)
	deleteParser.RegisterFlags(deleteCmd)

	if err := deleteParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

// deleteRequest bundles executeDelete's parameters below revive's
// argument-limit (5): component/stack/identity/target/force is 6 as separate
// parameters.
type deleteRequest struct {
	Component string
	Stack     string
	Identity  string
	Target    string
	Force     bool
}

func executeDelete(ctx context.Context, req deleteRequest) error {
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

	return prov.DeleteBackend(ctx, &DeleteBackendParams{
		CreateBackendParams: CreateBackendParams{
			AtmosConfig:     atmosConfig,
			Component:       req.Component,
			Stack:           req.Stack,
			ComponentConfig: componentConfig,
			AuthContext:     info.AuthContext,
			Target:          req.Target,
		},
		Force: req.Force,
	})
}
