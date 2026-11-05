package plugin

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/plugin/releaseplan"
)

func releasePlanCommand() *cobra.Command {
	parser := flags.NewStandardParser(
		flags.WithStringFlag("inventory", "", "plugins/releases.yaml", "Repository-relative release inventory"),
		flags.WithStringFlag("head", "", "HEAD", "Committed revision to plan"),
		flags.WithStringFlag("base", "", "", "Base revision for independent version checks"),
		flags.WithStringFlag("format", "", "json", "Output format (json)"),
		flags.WithEnvVars("inventory", "ATMOS_PLUGIN_RELEASE_INVENTORY"),
		flags.WithEnvVars("head", "ATMOS_PLUGIN_RELEASE_HEAD"),
		flags.WithEnvVars("base", "ATMOS_PLUGIN_RELEASE_BASE"),
	)
	command := &cobra.Command{Use: "plan", Short: "Plan independently versioned releases from committed manifests", Args: cobra.NoArgs}
	parser.RegisterFlags(command)
	command.RunE = func(cmd *cobra.Command, _ []string) error {
		values := viper.New()
		if err := parser.BindFlagsToViper(cmd, values); err != nil {
			return err
		}
		if values.GetString("format") != "json" {
			return fmt.Errorf("%w: release plans support --format json", errUtils.ErrPluginReleasePlan)
		}
		plan, err := createReleasePlan(cmd, values)
		if err != nil {
			return err
		}
		if err := writeReleasePlanOutputs(plan); err != nil {
			return err
		}
		return data.WriteJSON(plan)
	}
	return command
}

// writeReleasePlanOutputs exports matrices through the configured CI provider.
func writeReleasePlanOutputs(plan *releaseplan.Plan) error {
	if !ci.OutputEnabled(atmosConfig) {
		return nil
	}
	// Automatic outputs must not add local reporter renderings to command output.
	reporter := ci.NewReporter(atmosConfig).WithOutput(io.Discard)
	outputs := []struct {
		key   string
		value any
	}{
		{"packages", plan.Packages},
		{"build", plan.Build},
	}
	for _, output := range outputs {
		value, err := json.Marshal(output.value)
		if err != nil {
			return fmt.Errorf("%w: encode %s CI output: %w", errUtils.ErrPluginReleasePlan, output.key, err)
		}
		if _, err := reporter.Output(output.key, string(value)); err != nil {
			return err
		}
	}
	return nil
}
