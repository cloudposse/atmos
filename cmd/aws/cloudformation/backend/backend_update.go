package backend

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
)

var updateParser *flags.StandardParser

var updateCmd = &cobra.Command{
	Use:   "update [component]",
	Short: "Update the template-packaging backend bucket",
	Long: `Apply configuration changes to the existing S3 bucket referenced by the
component's "kind: aws/s3" provision target.

This operation is idempotent and updates bucket settings like versioning,
encryption, and public access blocking to match secure defaults.`,
	Example: `  atmos aws cloudformation backend update vpc --stack dev`,
	// Args validator is auto-set by parser via SetPositionalArgs with prompt-aware validation.
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		v := viper.GetViper()
		if err := updateParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}
		result, err := updateParser.Parse(ctx, args)
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
	updateCmd.DisableFlagParsing = false

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

	updateParser = flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithIdentityFlag(),
		flags.WithStringFlag("target", "", "", "The `kind: aws/s3` provision target to use. Required when more than one is declared."),
		flags.WithBoolFlag(flagAutoApprove, "", false, "Skip the confirmation prompt when the target bucket already exists."),
		flags.WithCompletionPrompt("stack", "Choose a stack", stackFlagCompletion),
		flags.WithPositionalArgPrompt("component", "Choose an aws/cloudformation component", componentArgCompletion),
	)

	updateParser.SetPositionalArgs(specs, nil, usage)
	updateParser.RegisterFlags(updateCmd)

	if err := updateParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
