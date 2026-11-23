package bootstrap

import (
	"context"
	"maps"
	"slices"

	"github.com/cloudposse/atmos/pkg/component/configuration"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/schema"
)

func init() {
	// The catalog reads declarations, the lock file, and local manifests; it never installs or starts a package.
	if err := configuration.RegisterProviderCatalog(ConfigurationProviders); err != nil {
		panic(err)
	}
}

// ConfigurationProviders lists the configuration providers that the enabled plugin packages export
// for component discovery. It reads the lock file for registry packages and the manifest for local
// builds, and never installs, restores, or starts a package. The complete result is false when an enabled
// package is neither locked nor local, because its exports are unknown until it is installed.
func ConfigurationProviders(ctx context.Context, atmos *schema.AtmosConfiguration) (map[string]bool, bool, error) {
	defer perf.Track(atmos, "pluginbootstrap.ConfigurationProviders")()

	names := map[string]bool{}
	aliases := enabledPackageAliases(atmos)
	if len(aliases) == 0 {
		return names, true, nil
	}
	state, err := readSelections(ctx, atmos)
	if err != nil {
		return nil, false, err
	}
	file, err := pluginlock.Load(ProjectDirectory(atmos))
	if err != nil {
		return nil, false, err
	}
	for _, alias := range aliases {
		exports, known, err := componentPackageExports(alias, state, file)
		if err != nil {
			return nil, false, err
		}
		if !known {
			return names, false, nil
		}
		for reference := range exports {
			if reference.Capability == extension.ComponentDiscovery {
				names[reference.Provider] = true
			}
		}
	}
	return names, true, nil
}

// enabledPackageAliases returns the sorted aliases of the enabled plugin packages.
func enabledPackageAliases(atmos *schema.AtmosConfiguration) []string {
	var aliases []string
	for _, alias := range slices.Sorted(maps.Keys(atmos.Plugins.Packages)) {
		declaration := atmos.Plugins.Packages[alias]
		if enabled(&declaration) {
			aliases = append(aliases, alias)
		}
	}
	return aliases
}
