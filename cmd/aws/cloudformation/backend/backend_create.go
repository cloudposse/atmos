package backend

import (
	"context"
	"fmt"

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
		autoApprove, autoApproveProvided := getCommandFlagBool(cmd, flagAutoApprove)
		if !autoApproveProvided {
			autoApprove = v.GetBool(flagAutoApprove)
		}
		return executeCreateOrUpdate(ctx, createOrUpdateArgs{
			Component:   result.Component,
			Stack:       stack,
			Identity:    identity,
			Target:      target,
			AutoApprove: autoApprove,
		})
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
		flags.WithBoolFlag(flagAutoApprove, "", false, "Skip the confirmation prompt when the target bucket already exists."),
		flags.WithCompletionPrompt("stack", "Choose a stack", stackFlagCompletion),
		flags.WithPositionalArgPrompt("component", "Choose an aws/cloudformation component", componentArgCompletion),
	)

	createParser.SetPositionalArgs(specs, nil, usage)
	createParser.RegisterFlags(createCmd)

	if err := createParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

// createOrUpdateArgs bundles executeCreateOrUpdate's inputs to stay under this
// repo's 5-argument function limit.
type createOrUpdateArgs struct {
	Component   string
	Stack       string
	Identity    string
	Target      string
	AutoApprove bool
}

// executeCreateOrUpdate is shared by `create` and `update`: both provision the
// backend via the same idempotent ProvisionWithParams code path. When the
// target bucket already exists, create/update always reconciles it to secure
// defaults (versioning, encryption, public-access-block, tags) — including
// replacing existing KMS encryption and tags — so unless AutoApprove is set,
// this prompts for confirmation before doing so, the same way every other
// mutating verb in this feature (apply, delete, changeset execute, stackset
// create/update/delete) already does.
func executeCreateOrUpdate(ctx context.Context, args createOrUpdateArgs) error {
	if args.Stack == "" {
		return errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
			WithExplanation("--stack flag is required").
			WithHint("Specify a stack with --stack or -s flag").
			Err()
	}

	atmosConfig, info, err := configInit.InitConfigAndAuth(args.Component, args.Stack, args.Identity)
	if err != nil {
		return err
	}

	componentConfig, err := configInit.DescribeComponent(atmosConfig, info, args.Component, args.Stack)
	if err != nil {
		return err
	}

	params := &CreateBackendParams{
		AtmosConfig:     atmosConfig,
		Component:       args.Component,
		Stack:           args.Stack,
		ComponentConfig: componentConfig,
		AuthContext:     info.AuthContext,
		Target:          args.Target,
	}

	if err := confirmExistingBackendOverwrite(ctx, params, args.AutoApprove); err != nil {
		return err
	}

	return prov.CreateBackend(ctx, params)
}

// confirmExistingBackendOverwrite prompts before reconciling an existing
// bucket's defaults, unless autoApprove is set or the bucket doesn't exist
// yet (a fresh create has nothing to confirm).
func confirmExistingBackendOverwrite(ctx context.Context, params *CreateBackendParams, autoApprove bool) error {
	if autoApprove {
		return nil
	}
	exists, err := prov.BackendExists(ctx, params)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	confirmed, err := flags.PromptForConfirmation(
		fmt.Sprintf("Bucket for %q already exists — applying secure defaults will overwrite its existing encryption, versioning, public-access, and tags. Continue?", params.Component),
		false,
	)
	if err != nil {
		return err
	}
	if !confirmed {
		return errUtils.ErrUserAborted
	}
	return nil
}
