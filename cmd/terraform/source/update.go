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

var updateParser *flags.StandardParser

var updateCmd = &cobra.Command{
	Use:   "update <component>",
	Short: "Re-vendor component source (force refresh)",
	Long: `Re-vendor a terraform component source, forcing a fresh download.

This command is equivalent to 'atmos terraform source create --force'. It downloads
the component source from the URI specified in metadata.source, replacing any existing
content in the component directory.`,
	Example: `  # Re-vendor component source
  atmos terraform source update vpc --stack dev`,
	Args: cobra.ExactArgs(1),
	RunE: executeUpdateCommand,
}

func init() {
	updateCmd.DisableFlagParsing = false

	updateParser = flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithIdentityFlag(),
	)

	updateParser.RegisterFlags(updateCmd)

	if err := updateParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func executeUpdateCommand(cmd *cobra.Command, args []string) error {
	defer perf.Track(atmosConfigPtr, "source.update.RunE")()

	component := args[0]

	// Parse common flags.
	opts, err := ParseCommonFlags(cmd, updateParser)
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

	// Check if metadata.source is configured.
	if !source.HasMetadataSource(componentConfig) {
		return errUtils.Build(errUtils.ErrMetadataSourceMissing).
			WithContext("component", component).
			WithContext("stack", opts.Stack).
			WithHint("Add metadata.source to the component configuration in your stack manifest").
			Err()
	}

	// Provision the source with force=true.
	ctx := context.Background()
	return ProvisionSource(ctx, &ProvisionSourceOptions{
		AtmosConfig:     atmosConfig,
		Component:       component,
		Stack:           opts.Stack,
		ComponentConfig: componentConfig,
		AuthContext:     authContext,
		Force:           true,
	})
}
