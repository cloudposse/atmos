package bootstrap

import (
	"errors"
	"maps"
	"slices"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/plugin/install"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	pluginplatform "github.com/cloudposse/atmos/pkg/plugin/platform"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// announcedLocal remembers which aliases already printed the local-path warning for a configuration,
// so one invocation warns once per alias even though several steps register the same selection.
var announcedLocal = struct {
	sync.Mutex
	seen map[*schema.AtmosConfiguration]map[string]bool
}{seen: make(map[*schema.AtmosConfiguration]map[string]bool)}

// announceLocal warns once per alias that a package runs from a local path. A path package skips the
// lock, the cache, signature evidence and version selection, so the warning says so loudly.
func announceLocal(atmos *schema.AtmosConfiguration, state *config.Selections) {
	announcedLocal.Lock()
	defer announcedLocal.Unlock()
	seen := announcedLocal.seen[atmos]
	if seen == nil {
		seen = make(map[string]bool)
		announcedLocal.seen[atmos] = seen
	}
	for _, alias := range slices.Sorted(maps.Keys(state.Packages)) {
		entry := state.Packages[alias]
		if !entry.Local || seen[alias] {
			continue
		}
		seen[alias] = true
		ui.Warningf("Plugin `%s` runs from local path `%s`; lock, signature and version checks are bypassed.", alias, config.DisplayPath(entry.Path))
	}
}

// loadLocalManifest reads a selected package's manifest. A failure for a path package names the
// alias and directory, because the manifest is a build output the user controls.
func loadLocalManifest(alias string, entry *config.Selection) (*manifest.Config, error) {
	definition, err := manifest.Load(entry.Path)
	if err != nil && entry.Local {
		return nil, localPathError(alias, entry.Path, err)
	}
	if err == nil && entry.Local && len(definition.Spec.Platforms) > 0 {
		_, err = pluginplatform.Select(definition.Spec.Platforms, install.CurrentPlatform())
	}
	return definition, err
}

// localPathError explains a path package that cannot be activated and how to repair it.
func localPathError(alias, path string, err error) error {
	return errUtils.Build(err).
		WithHintf("Package `%s` runs from `plugins.packages.%s.path`, which resolves to `%s`. Check that the directory holds `plugin.yaml` and the built executable, and run `atmos plugin build %s` to compile it.", alias, alias, config.DisplayPath(path), config.DisplayPath(path)).
		Err()
}

// repairLocal adds the repair hint to a path package failure that a configuration or build problem
// causes. Policy denials and cancellations keep their own diagnostics.
func repairLocal(alias string, entry *config.Selection, err error) error {
	if entry.Local && (errors.Is(err, errUtils.ErrPluginInvalidConfig) || errors.Is(err, errUtils.ErrPluginNotInstalled)) {
		return localPathError(alias, entry.Path, err)
	}
	return err
}

// selectedExecutable returns the executable to start. A path package reads it from the manifest it
// just loaded, so rebuilding or editing the manifest is picked up by the next command.
func selectedExecutable(entry *config.Selection, definition *manifest.Config) string {
	if entry.Local {
		return manifest.Executable(definition, install.CurrentPlatform())
	}
	return entry.Executable
}
