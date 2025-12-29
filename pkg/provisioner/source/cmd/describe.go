package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/source"
)

// DescribeCommand creates a describe command for the given component type.
func DescribeCommand(cfg *Config) *cobra.Command {
	parser := flags.NewStandardParser(
		flags.WithStackFlag(),
	)

	cmd := &cobra.Command{
		Use:   "describe <component>",
		Short: fmt.Sprintf("Show source configuration for a %s component", cfg.TypeLabel),
		Long: fmt.Sprintf(`Display the source configuration for a %s component.

This command shows the source URI, version, and any path filters configured
for the component. Output matches the stack manifest schema format.`, cfg.TypeLabel),
		Example: fmt.Sprintf(`  # Describe source configuration
  atmos %s source describe vpc --stack dev`, cfg.ComponentType),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeDescribe(cmd, args, cfg, parser)
		},
	}

	cmd.DisableFlagParsing = false
	parser.RegisterFlags(cmd)

	if err := parser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	return cmd
}

func executeDescribe(cmd *cobra.Command, args []string, cfg *Config, parser *flags.StandardParser) error {
	defer perf.Track(nil, fmt.Sprintf("source.%s.describe.RunE", cfg.ComponentType))()

	component := args[0]

	// Parse flags.
	v := viper.GetViper()
	if err := parser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	stack := v.GetString("stack")
	if stack == "" {
		return errUtils.ErrRequiredFlagNotProvided
	}

	// Get component configuration.
	componentConfig, err := DescribeComponent(component, stack)
	if err != nil {
		return errUtils.Build(errUtils.ErrDescribeComponent).
			WithCause(err).
			WithContext("component", component).
			WithContext("stack", stack).
			Err()
	}

	// Extract source configuration.
	sourceSpec, err := source.ExtractSource(componentConfig)
	if err != nil {
		return err
	}

	if sourceSpec == nil {
		return errUtils.Build(errUtils.ErrSourceMissing).
			WithContext("component", component).
			WithContext("stack", stack).
			WithHint("Add source to the component configuration in your stack manifest").
			Err()
	}

	// Output as YAML matching stack manifest schema format.
	output := map[string]any{
		"components": map[string]any{
			cfg.ComponentType: map[string]any{
				component: map[string]any{
					"source": sourceSpec,
				},
			},
		},
	}

	return data.WriteYAML(output)
}
