package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// newAuthConfigAndStacksInfo builds a ConfigAndStacksInfo populated from the
// global CLI flags (--base-path, --config, --config-path). Auth subcommands
// call InitCliConfig without stack manifests, but they still need to honor
// the user's config-selection flags so that atmos.yaml resolution matches
// what the root command already loaded.
//
// Without this, calls like `atmos --config custom.yaml auth login` would
// silently fall back to the default atmos.yaml because the subcommand
// passes an empty ConfigAndStacksInfo.
func newAuthConfigAndStacksInfo(cmd *cobra.Command) schema.ConfigAndStacksInfo {
	defer perf.Track(nil, "cmd.newAuthConfigAndStacksInfo")()

	globalFlags := flags.ParseGlobalFlags(cmd, viper.GetViper())
	return schema.ConfigAndStacksInfo{
		AtmosBasePath:           globalFlags.BasePath,
		AtmosConfigFilesFromArg: globalFlags.Config,
		AtmosConfigDirsFromArg:  globalFlags.ConfigPath,
	}
}
