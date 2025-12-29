package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/source"
)

// PullCommand creates a pull command for the given component type.
func PullCommand(cfg *Config) *cobra.Command {
	parser := flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithIdentityFlag(),
		flags.WithBoolFlag("force", "f", false, "Force re-vendor even if component directory exists"),
	)

	cmd := &cobra.Command{
		Use:   "pull <component>",
		Short: fmt.Sprintf("Vendor %s component source from source configuration", cfg.TypeLabel),
		Long: fmt.Sprintf(`Vendor a %s component source based on source configuration.

This command downloads the component source from the URI specified in the source field
and places it in the appropriate component directory. The source can be any go-getter
compatible URI (git, s3, http, oci, etc.).

If the component is already vendored, it will be skipped unless --force is specified.`, cfg.TypeLabel),
		Example: fmt.Sprintf(`  # Vendor component source (downloads if missing or outdated)
  atmos %s source pull vpc --stack dev

  # Force re-vendor even if up-to-date
  atmos %s source pull vpc --stack dev --force`, cfg.ComponentType, cfg.ComponentType),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return executePull(cmd, args, cfg, parser)
		},
	}

	cmd.DisableFlagParsing = false
	parser.RegisterFlags(cmd)

	if err := parser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	return cmd
}

func executePull(cmd *cobra.Command, args []string, cfg *Config, parser *flags.StandardParser) error {
	defer perf.Track(nil, fmt.Sprintf("source.%s.pull.RunE", cfg.ComponentType))()

	component := args[0]

	// Parse common flags.
	opts, err := ParseCommonFlags(cmd, parser)
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
		return errUtils.Build(errUtils.ErrSourceMissing).
			WithContext("component", component).
			WithContext("stack", opts.Stack).
			WithHint("Add source to the component configuration in your stack manifest").
			Err()
	}

	// Provision the source using command context for cancellation propagation.
	return ProvisionSource(cmd.Context(), &ProvisionSourceOptions{
		AtmosConfig:     atmosConfig,
		ComponentType:   cfg.ComponentType,
		Component:       component,
		Stack:           opts.Stack,
		ComponentConfig: componentConfig,
		AuthContext:     authContext,
		Force:           opts.Force,
	})
}
