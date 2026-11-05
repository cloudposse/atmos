package bootstrap

import (
	"context"
	"maps"
	"slices"

	"github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/plugin/install"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	pluginpolicy "github.com/cloudposse/atmos/pkg/plugin/policy"
	"github.com/cloudposse/atmos/pkg/policy/guard"
	"github.com/cloudposse/atmos/pkg/schema"
)

// admitLocal checks every path package against the current policy before its manifest is trusted
// for activation. The policy input carries the plugin directory as input.path.
func admitLocal(ctx context.Context, atmos *schema.AtmosConfiguration, state *config.Selections) error {
	enabled, err := guard.Enabled("plugin")
	if err != nil || !enabled {
		return err
	}
	registry, err := Descriptors()
	if err != nil {
		return err
	}
	for _, alias := range slices.Sorted(maps.Keys(state.Packages)) {
		entry := state.Packages[alias]
		if !entry.Local {
			continue
		}
		path := pluginpolicy.DevelopmentPath(ProjectDirectory(atmos), entry.Path)
		if err := pluginpolicy.Source(ctx, "activate", "", pluginpolicy.WithPath(path)); err != nil {
			return err
		}
		definition, err := loadLocalManifest(alias, &entry)
		if err != nil {
			return err
		}
		if err := pluginpolicy.Package(ctx, "activate", "", definition, pluginpolicy.WithExtensions(registry), pluginpolicy.WithPath(path)); err != nil {
			return err
		}
	}
	return nil
}

func admitInstalled(ctx context.Context, declarations map[string]schema.PluginPackage, file *pluginlock.File, cache *install.Cache) error {
	enabled, err := guard.Enabled("plugin")
	if err != nil || !enabled {
		return err
	}
	registry, err := Descriptors()
	if err != nil {
		return err
	}
	for alias := range declarations {
		entry := file.Spec.Packages[alias]
		if err := cache.CheckPolicy(ctx, "activate", &entry, install.CurrentPlatform(), registry); err != nil {
			return err
		}
	}
	return nil
}
