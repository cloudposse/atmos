package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// newAuthConfigAndStacksInfo builds a ConfigAndStacksInfo populated from the
// global CLI flags (--base-path, --config, --config-path, --profile). Auth
// subcommands call InitCliConfig without stack manifests, but they still need
// to honor the user's config-selection flags so that atmos.yaml resolution
// matches what the root command already loaded.
//
// Without this, calls like `atmos --config custom.yaml auth login` would
// silently fall back to the default atmos.yaml because the subcommand
// passes an empty ConfigAndStacksInfo.
//
// Propagating --profile is especially important for the profile-fallback
// re-exec path: when `atmos auth exec` with no identity triggers the fallback
// and the user picks a profile, the re-exec runs `atmos --profile <name> auth
// exec`. If ProfilesFromArg is not populated here, InitCliConfig loads only
// the base atmos.yaml and the picked profile's identities never make it into
// the auth manager — producing the same "no identities available" error the
// fallback was meant to resolve.
func newAuthConfigAndStacksInfo(cmd *cobra.Command) schema.ConfigAndStacksInfo {
	defer perf.Track(nil, "cmd.newAuthConfigAndStacksInfo")()

	globalFlags := flags.ParseGlobalFlags(cmd, viper.GetViper())
	return schema.ConfigAndStacksInfo{
		AtmosBasePath:           globalFlags.BasePath,
		AtmosConfigFilesFromArg: globalFlags.Config,
		AtmosConfigDirsFromArg:  globalFlags.ConfigPath,
		ProfilesFromArg:         globalFlags.Profile,
	}
}
