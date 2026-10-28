package config

import (
	"slices"

	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	DefaultRegistryAlias = "cloudposse"
	DefaultCatalog       = "oci://ghcr.io/cloudposse/atmos/plugins/catalog:stable"
)

// EffectiveRegistries returns isolated discovery defaults and explicit overrides.
// An explicit alias replaces the whole built-in entry, including its catalog.
//
//nolint:lintroller // Pure configuration composition without I/O.
func EffectiveRegistries(plugins *schema.PluginsConfig) map[string]schema.PluginRegistry {
	registries := make(map[string]schema.PluginRegistry)
	if plugins.Discovery.DefaultRegistry == nil || *plugins.Discovery.DefaultRegistry {
		registries[DefaultRegistryAlias] = schema.PluginRegistry{Catalog: DefaultCatalog}
	}
	for alias, registry := range plugins.Registries {
		registry.Repositories = slices.Clone(registry.Repositories)
		registries[alias] = registry
	}
	return registries
}
