package plugin

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
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
		plan, err := releaseplan.Create(cmd.Context(), releaseplan.WithInventory(values.GetString("inventory")), releaseplan.WithHead(values.GetString("head")), releaseplan.WithBase(values.GetString("base")))
		if err != nil {
			return err
		}
		return data.WriteJSON(plan)
	}
	return command
}
