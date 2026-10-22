package bootstrap

import (
	"context"
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/plugin/install"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/schema"
)

func selectedPackages(ctx context.Context, atmos *schema.AtmosConfiguration, aliases ...string) (*config.Selections, error) {
	state, err := readSelections(ctx, atmos)
	if err != nil {
		return nil, err
	}
	if len(aliases) != 0 {
		atmos = selectAliases(atmos, state, aliases)
	}
	if len(activeDeclarations(atmos, state)) == 0 && len(state.Packages) == 0 {
		return state, nil
	}
	// A path package has no signature, so plugins.policy.require_signature governs released packages
	// only; policy rules over origin development and input.path are the gate for local builds.
	if err := admitLocal(ctx, atmos, state); err != nil {
		return nil, err
	}
	return selectLockedPackages(ctx, atmos, state)
}

// readSelections validates the configuration and resolves the packages that run from a local path.
func readSelections(ctx context.Context, atmos *schema.AtmosConfiguration) (*config.Selections, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := config.Validate(&atmos.Plugins); err != nil {
		return nil, err
	}
	if err := config.CheckTransaction(ProjectDirectory(atmos)); err != nil {
		return nil, err
	}
	return config.LocalSelections(ProjectDirectory(atmos), atmos.Plugins.Packages)
}

func lockedSelection(alias string, declaration *schema.PluginPackage, file *pluginlock.File, cache *install.Cache) (config.Selection, error) {
	entry, found := file.Spec.Packages[alias]
	if !found {
		return config.Selection{}, fmt.Errorf("%w: %s", errUtils.ErrPluginNotInstalled, alias)
	}
	if entry.Source != declaration.Source || entry.Constraint != declaration.Version {
		return config.Selection{}, fmt.Errorf("%w: %s", errUtils.ErrPluginFrozenDrift, alias)
	}
	directory, err := cache.Installed(&entry, install.CurrentPlatform())
	if errors.Is(err, errUtils.ErrPluginCacheCorrupt) {
		// Corruption is not absence: never run it and never skip it as "not installed".
		return config.Selection{}, cache.CorruptReleaseError(err, alias, &entry, install.CurrentPlatform(), install.RepairAutomatic)
	}
	if err != nil {
		return config.Selection{}, fmt.Errorf("%w: %s: %w", errUtils.ErrPluginNotInstalled, alias, err)
	}
	return config.Selection{Path: directory, Executable: entry.Runtime.Executable}, nil
}

func applySelections(atmos *schema.AtmosConfiguration, state *config.Selections, file *pluginlock.File, cache *install.Cache) (*config.Selections, error) {
	for alias, declaration := range atmos.Plugins.Packages {
		if declaration.Enabled != nil && !*declaration.Enabled {
			delete(state.Packages, alias)
			continue
		}
		if _, local := state.Packages[alias]; local {
			continue
		}
		selected, err := lockedSelection(alias, &declaration, file, cache)
		if err != nil {
			return nil, err
		}
		state.Packages[alias] = selected
	}
	return state, nil
}

func selectLockedPackages(ctx context.Context, atmos *schema.AtmosConfiguration, state *config.Selections) (*config.Selections, error) {
	declarations := activeDeclarations(atmos, state)
	if len(declarations) == 0 {
		return state, nil
	}
	file, err := pluginlock.Load(ProjectDirectory(atmos))
	if err != nil {
		return nil, err
	}
	cache, err := install.NewCache(atmos.Plugins.CacheDir)
	if err != nil {
		return nil, err
	}
	if err := ensureSelections(ctx, atmos, declarations, file, cache); err != nil {
		return nil, err
	}
	file, err = pluginlock.Load(ProjectDirectory(atmos))
	if err != nil {
		return nil, err
	}
	if err := admitInstalled(ctx, declarations, file, cache); err != nil {
		return nil, err
	}
	if err := verifySelections(ctx, atmos, declarations, file); err != nil {
		return nil, err
	}
	return applySelections(atmos, state, file, cache)
}

func verifySelections(ctx context.Context, atmos *schema.AtmosConfiguration, declarations map[string]schema.PluginPackage, file *pluginlock.File) error {
	if install.SignatureRequired(&atmos.Plugins.Policy) {
		descriptors, err := Descriptors()
		if err != nil {
			return err
		}
		service, err := install.NewService(atmos, descriptors)
		if err != nil {
			return err
		}
		for alias := range declarations {
			entry := file.Spec.Packages[alias]
			if _, err := service.VerifyInstalled(ctx, &entry); err != nil {
				return err
			}
		}
	}
	return nil
}

func activeDeclarations(atmos *schema.AtmosConfiguration, state *config.Selections) map[string]schema.PluginPackage {
	result := make(map[string]schema.PluginPackage)
	for alias, declaration := range atmos.Plugins.Packages {
		if declaration.Enabled != nil && !*declaration.Enabled {
			delete(state.Packages, alias)
			continue
		}
		if _, local := state.Packages[alias]; !local {
			result[alias] = declaration
		}
	}
	return result
}

// selectAliases restricts the configuration and the local selections to the named aliases.
func selectAliases(atmos *schema.AtmosConfiguration, state *config.Selections, aliases []string) *schema.AtmosConfiguration {
	selected := *atmos
	selected.Plugins.Packages = make(map[string]schema.PluginPackage)
	overrides := make(map[string]config.Selection)
	for _, alias := range aliases {
		if declaration, found := atmos.Plugins.Packages[alias]; found {
			selected.Plugins.Packages[alias] = declaration
		}
		if override, found := state.Packages[alias]; found {
			overrides[alias] = override
		}
	}
	state.Packages = overrides
	return &selected
}
