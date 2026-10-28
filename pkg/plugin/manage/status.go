package manage

import (
	"errors"
	"fmt"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/plugin/install"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Status is a credential-free view of a selected package.
type Status struct {
	Alias string `json:"alias" yaml:"alias"`
	State string `json:"state" yaml:"state"`
	Path  string `json:"path,omitempty" yaml:"path,omitempty"`
	// Source is "path" when plugins.packages.<alias>.path selects a local build, and empty for releases.
	Source string `json:"source,omitempty" yaml:"source,omitempty"`
	// Plugin and Version come from the local manifest of a path package; releases report them in Release.
	Plugin  string `json:"plugin,omitempty" yaml:"plugin,omitempty"`
	Version string `json:"version,omitempty" yaml:"version,omitempty"`
	// Hint explains how to resolve a state that needs attention, such as a corrupt cached release.
	Hint     string            `json:"hint,omitempty" yaml:"hint,omitempty"`
	Release  *pluginlock.Entry `json:"release,omitempty" yaml:"release,omitempty"`
	Manifest *manifest.Config  `json:"manifest,omitempty" yaml:"manifest,omitempty"`
}

// List reports release selections and path packages without networking.
func List(atmos *schema.AtmosConfiguration, project string) ([]Status, error) {
	defer perf.Track(atmos, "pluginmanage.List")()

	// Reject what ordinary commands reject, instead of reporting "No plugins configured".
	if err := config.Validate(&atmos.Plugins); err != nil {
		return nil, err
	}
	file, err := pluginlock.Load(project)
	if err != nil {
		return nil, err
	}
	local, err := config.LocalSelections(project, atmos.Plugins.Packages)
	if err != nil {
		return nil, err
	}
	aliases := make(map[string]bool)
	for alias := range atmos.Plugins.Packages {
		aliases[alias] = true
	}
	cache, err := install.NewCache(atmos.Plugins.CacheDir)
	if err != nil {
		return nil, err
	}
	result := make([]Status, 0, len(aliases))
	for alias := range aliases {
		status := packageStatus(alias, atmos, file, local, cache)
		result = append(result, status)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Alias < result[j].Alias })
	return result, nil
}

// Info adds the package's static metadata to its local installation status.
func Info(atmos *schema.AtmosConfiguration, project, alias string) (*Status, error) {
	defer perf.Track(atmos, "pluginmanage.Info")()

	installed, err := List(atmos, project)
	if err != nil {
		return nil, err
	}
	for index := range installed {
		status := &installed[index]
		if status.Alias != alias {
			continue
		}
		if status.Path != "" && status.State != "broken" {
			status.Manifest, err = manifest.Load(status.Path)
			if err != nil {
				return nil, err
			}
		}
		return status, nil
	}
	return nil, errUtils.ErrPluginNotInstalled
}

func packageStatus(alias string, atmos *schema.AtmosConfiguration, file *pluginlock.File, local *config.Selections, cache *install.Cache) Status {
	status := Status{Alias: alias, State: "missing"}
	if entry, found := file.Spec.Packages[alias]; found {
		status.Release = &entry
		directory, err := cache.Installed(&entry, install.CurrentPlatform())
		switch {
		case err == nil:
			status.State, status.Path = "installed", directory
		case errors.Is(err, errUtils.ErrPluginCacheCorrupt):
			status.State, status.Hint = "corrupt", corruptHint(cache, alias, &entry)
		}
	}
	if selection, found := local.Packages[alias]; found {
		localStatus(&status, &selection)
	}
	if declaration, found := atmos.Plugins.Packages[alias]; found && declaration.Enabled != nil && !*declaration.Enabled {
		status.State = "disabled"
	}
	return status
}

// localStatus reports a path package from its manifest. A manifest that cannot be read is "broken"
// and says how to repair it; the package never falls back to the release it replaces.
func localStatus(status *Status, selection *config.Selection) {
	status.State, status.Path, status.Source, status.Hint = "path", selection.Path, "path", ""
	definition, err := manifest.Load(selection.Path)
	if err != nil {
		status.State = "broken"
		status.Hint = fmt.Sprintf("Plugin `%s` runs from `plugins.packages.%s.path` (`%s`), but its `plugin.yaml` cannot be read. Check the path, or run `atmos plugin build %s` there.", status.Alias, status.Alias, config.DisplayPath(selection.Path), config.DisplayPath(selection.Path))
		return
	}
	status.Plugin, status.Version = definition.Metadata.Name, definition.Metadata.Version
}

// corruptHint names the alias, the cache path, and the command that repairs the release.
func corruptHint(cache *install.Cache, alias string, entry *pluginlock.Entry) string {
	directory, err := cache.ReleaseDirectory(entry, install.CurrentPlatform())
	if err != nil {
		directory = cache.Directory
	}
	return fmt.Sprintf("The cached release of plugin `%s` at `%s` no longer matches its locked digest. Run `atmos plugin install` to quarantine it and re-download it.", alias, directory)
}
