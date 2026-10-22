package bootstrap

import (
	"context"
	"errors"
	"os"
	"sync"

	authregistry "github.com/cloudposse/atmos/pkg/auth/registry"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/plugin/install"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/schema"
)

// PrepareAvailable temporarily registers verified local auth plugins needed by
// configured registry identities. It never restores packages or contacts a registry.
func PrepareAvailable(ctx context.Context, atmos *schema.AtmosConfiguration, options ...Option) (func(), error) {
	defer perf.Track(atmos, "pluginbootstrap.PrepareAvailable")()

	availableAuthMu.Lock()
	defer availableAuthMu.Unlock()
	if active := availableAuthRegistrations[atmos]; active != nil {
		active.users++
		return releaseAvailable(atmos, active), nil
	}
	kinds := registryAuthKinds(atmos)
	if len(kinds) == 0 {
		return func() {}, nil
	}
	state, err := availableAuthPackages(ctx, atmos, kinds)
	if err != nil {
		return nil, err
	}
	cleanup, err := registerSelected(ctx, atmos, state, options...)
	if err != nil {
		return nil, err
	}
	active := &availableRegistration{cleanup: cleanup, users: 1}
	availableAuthRegistrations[atmos] = active
	return releaseAvailable(atmos, active), nil
}

type availableRegistration struct {
	cleanup func()
	users   int
}

var (
	availableAuthMu            sync.Mutex
	availableAuthRegistrations = make(map[*schema.AtmosConfiguration]*availableRegistration)
)

func releaseAvailable(atmos *schema.AtmosConfiguration, active *availableRegistration) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			availableAuthMu.Lock()
			defer availableAuthMu.Unlock()
			active.users--
			if active.users == 0 {
				active.cleanup()
				delete(availableAuthRegistrations, atmos)
			}
		})
	}
}

func availableAuthPackages(ctx context.Context, atmos *schema.AtmosConfiguration, kinds map[string]bool) (*config.Selections, error) {
	state, err := availableLocal(ctx, atmos, kinds)
	if err != nil {
		return nil, err
	}
	file, err := pluginlock.Load(ProjectDirectory(atmos))
	if err != nil {
		return nil, err
	}
	cache, err := install.NewCache(atmos.Plugins.CacheDir)
	if err != nil {
		return nil, err
	}
	selected, err := availableDeclarations(atmos, state, file, cache, kinds)
	if err != nil {
		return nil, err
	}
	if err := admitInstalled(ctx, selected.Plugins.Packages, file, cache); err != nil {
		return nil, err
	}
	// Bootstrap cannot depend on a signature download authenticated by the plugin
	// being bootstrapped. Its evidence and trust root must already be available.
	selected.Plugins.Policy.Offline = true
	if err := verifySelections(ctx, selected, selected.Plugins.Packages, file); err != nil {
		return nil, err
	}
	return applySelections(selected, state, file, cache)
}

func availableLocal(ctx context.Context, atmos *schema.AtmosConfiguration, kinds map[string]bool) (*config.Selections, error) {
	state, err := readSelections(ctx, atmos)
	if err != nil {
		return nil, err
	}
	activeDeclarations(atmos, state) // Remove disabled packages before opening local manifests.
	if err := admitLocal(ctx, atmos, state); err != nil {
		return nil, err
	}
	if err := filterLocalAuth(state, kinds); err != nil {
		return nil, err
	}
	return state, nil
}

func availableDeclarations(atmos *schema.AtmosConfiguration, state *config.Selections, file *pluginlock.File, cache *install.Cache, kinds map[string]bool) (*schema.AtmosConfiguration, error) {
	selected := *atmos
	selected.Plugins.Packages = make(map[string]schema.PluginPackage)
	for alias, declaration := range activeDeclarations(atmos, state) {
		entry, exists := file.Spec.Packages[alias]
		if !exists || !requiredAuthExport(entry.Exports, kinds) {
			continue
		}
		if _, err := lockedSelection(alias, &declaration, file, cache); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		selected.Plugins.Packages[alias] = declaration
	}
	return &selected, nil
}

func requiredAuthExport(exports []pluginlock.Export, kinds map[string]bool) bool {
	for _, export := range exports {
		if (export.Capability == "auth.provider" || export.Capability == "auth.identity") && kinds[export.Name] {
			return true
		}
	}
	return false
}

func filterLocalAuth(state *config.Selections, kinds map[string]bool) error {
	registry, err := Descriptors()
	if err != nil {
		return err
	}
	for alias, selection := range state.Packages {
		definition, err := loadLocalManifest(alias, &selection)
		if err != nil {
			return err
		}
		bindings, err := registry.Discover(definition.Spec.Capabilities)
		if err != nil {
			return err
		}
		exports := make([]pluginlock.Export, 0, len(bindings))
		for _, binding := range bindings {
			exports = append(exports, pluginlock.Export{Capability: binding.Capability, Name: binding.Name})
		}
		if !requiredAuthExport(exports, kinds) {
			delete(state.Packages, alias)
		}
	}
	return nil
}

func registryAuthKinds(atmos *schema.AtmosConfiguration) map[string]bool {
	kinds := make(map[string]bool)
	seen := make(map[string]bool)
	for _, registry := range atmos.Plugins.Registries {
		collectIdentityKinds(atmos, registry.Identity, kinds, seen)
	}
	return kinds
}

func collectIdentityKinds(atmos *schema.AtmosConfiguration, current string, kinds, seen map[string]bool) {
	for current != "" && !seen[current] {
		seen[current] = true
		identity, found := atmos.Auth.Identities[current]
		if !found {
			return
		}
		if !authregistry.HasIdentity(identity.Kind) {
			kinds[identity.Kind] = true
		}
		provider := identity.Provider
		current = ""
		if identity.Via != nil {
			current = identity.Via.Identity
			if identity.Via.Provider != "" {
				provider = identity.Via.Provider
			}
		}
		if item, exists := atmos.Auth.Providers[provider]; exists && !authregistry.HasProvider(item.Kind) {
			kinds[item.Kind] = true
		}
	}
}
