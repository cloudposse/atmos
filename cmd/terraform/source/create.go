package source

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/source"
)

var createParser *flags.StandardParser

var createCmd = &cobra.Command{
	Use:   "create <component>",
	Short: "Vendor component source from source configuration",
	Long: `Vendor a terraform component source based on source configuration.

This command downloads the component source from the URI specified in the source field
and places it in the appropriate component directory. The source can be any go-getter
compatible URI (git, s3, http, oci, etc.).

If the component directory already exists, use --force to re-vendor.`,
	Example: `  # Vendor component source
  atmos terraform source create vpc --stack dev

  # Force re-vendor even if directory exists
  atmos terraform source create vpc --stack dev --force`,
	Args: cobra.ExactArgs(1),
	RunE: executeCreateCommand,
}

func init() {
	createCmd.DisableFlagParsing = false

	createParser = flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithIdentityFlag(),
		flags.WithBoolFlag("force", "f", false, "Force re-vendor even if component directory exists"),
	)

	createParser.RegisterFlags(createCmd)

	if err := createParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func executeCreateCommand(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "source.create.RunE")()

	component := args[0]

	// Parse common flags.
	opts, err := ParseCommonFlags(cmd, createParser)
	if err != nil {
		return err
	}

	// Initialize config and auth with global flags.
	atmosConfig, authContext, err := InitConfigAndAuth(component, opts.Stack, opts.Identity, &opts.Flags)
	if err != nil {
		return err
	}

	// Get component configuration.
	componentConfig, err := DescribeComponent(component, opts.Stack)
	if err != nil {
		return errUtils.Build(errUtils.ErrDescribeComponent).
			WithCause(err).
			WithContext("component", component).
			WithContext("stack", opts.Stack).
			Err()
	}

	// Check if source is configured.
	if !source.HasSource(componentConfig) {
		return errUtils.Build(errUtils.ErrMetadataSourceMissing).
			WithContext("component", component).
			WithContext("stack", opts.Stack).
			WithHint("Add source to the component configuration in your stack manifest").
			Err()
	}

	// Provision the source.
	ctx := context.Background()
	return ProvisionSource(ctx, &ProvisionSourceOptions{
		AtmosConfig:     atmosConfig,
		Component:       component,
		Stack:           opts.Stack,
		ComponentConfig: componentConfig,
		AuthContext:     authContext,
		Force:           opts.Force,
	})
}
