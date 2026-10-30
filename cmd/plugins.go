package cmd

import (
	"github.com/spf13/cobra"

	plugincmd "github.com/cloudposse/atmos/cmd/plugin"
	"github.com/cloudposse/atmos/pkg/plugin/bootstrap"
	"github.com/cloudposse/atmos/pkg/policy/guard"
	"github.com/cloudposse/atmos/pkg/schema"
)

var releasePlugins func()

// preparePluginRegistry is the composition seam for command tests; production uses Bootstrap.
var preparePluginRegistry = bootstrap.Prepare

func clearPlugins() {
	if releasePlugins != nil {
		releasePlugins()
		releasePlugins = nil
	}
	plugincmd.SetAtmosConfig(nil)
	guard.SetActiveProfiles(nil)
}

// preparePlugins registers the plugin exports the configuration references before auth.
// It installs a missing package only when a referenced extension kind needs it, never
// for commands such as describe or list whose configuration references no plugin kind
// (see bootstrap.Prepare). Lifecycle and repair commands remain usable when an installed
// plugin is broken.
func preparePlugins(command *cobra.Command, config *schema.AtmosConfiguration) error {
	clearPlugins()
	plugincmd.SetAtmosConfig(config)
	// Plugin policy rules see the profiles this invocation applied as input.profiles.
	guard.SetActiveProfiles(config.ActiveProfiles)
	for current := command; current != nil; current = current.Parent() {
		if isTopLevelCommand(current) && (current.Name() == "plugin" || current.Name() == "version") {
			return nil
		}
	}
	cleanup, err := preparePluginRegistry(command.Context(), config)
	if err != nil {
		return err
	}
	releasePlugins = cleanup
	return nil
}
