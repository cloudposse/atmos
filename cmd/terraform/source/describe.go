package source

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/source"
)

var describeParser *flags.StandardParser

var describeCmd = &cobra.Command{
	Use:   "describe <component>",
	Short: "Show source configuration for a component",
	Long: `Display the source configuration for a terraform component.

This command shows the source URI, version, and any path filters configured
for the component. Output matches the stack manifest schema format.`,
	Example: `  # Describe source configuration
  atmos terraform source describe vpc --stack dev`,
	Args: cobra.ExactArgs(1),
	RunE: executeDescribeCommand,
}

func init() {
	describeCmd.DisableFlagParsing = false

	describeParser = flags.NewStandardParser(
		flags.WithStackFlag(),
	)

	describeParser.RegisterFlags(describeCmd)

	if err := describeParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func executeDescribeCommand(cmd *cobra.Command, args []string) error {
	defer perf.Track(atmosConfigPtr, "source.describe.RunE")()

	component := args[0]

	// Parse flags.
	v := viper.GetViper()
	if err := describeParser.BindFlagsToViper(cmd, v); err != nil {
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
		return errUtils.Build(errUtils.ErrMetadataSourceMissing).
			WithContext("component", component).
			WithContext("stack", stack).
			WithHint("Add source to the component configuration in your stack manifest").
			Err()
	}

	// Output as YAML matching stack manifest schema format.
	output := map[string]any{
		"components": map[string]any{
			"terraform": map[string]any{
				component: map[string]any{
					"source": sourceSpec,
				},
			},
		},
	}

	return data.WriteYAML(output)
}
