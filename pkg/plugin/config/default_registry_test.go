package config

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestEffectiveRegistries(t *testing.T) {
	for _, enabled := range []*bool{nil, new(true), new(false)} {
		settings := schema.PluginsConfig{Discovery: schema.PluginDiscoveryConfig{DefaultRegistry: enabled}}
		result := EffectiveRegistries(&settings)
		if enabled != nil && !*enabled {
			require.Empty(t, result)
		} else {
			require.Equal(t, map[string]schema.PluginRegistry{DefaultRegistryAlias: {Catalog: DefaultCatalog}}, result)
		}
		require.Nil(t, settings.Registries)
	}
	settings := schema.PluginsConfig{Registries: map[string]schema.PluginRegistry{
		DefaultRegistryAlias: {Host: "example.com", Repositories: []string{"oci://example.com/one", "oci://example.com/two"}},
	}}
	result := EffectiveRegistries(&settings)
	require.Empty(t, result[DefaultRegistryAlias].Catalog)
	require.Equal(t, "example.com", result[DefaultRegistryAlias].Host)
	result[DefaultRegistryAlias].Repositories[0] = "modified"
	require.Equal(t, "oci://example.com/one", settings.Registries[DefaultRegistryAlias].Repositories[0])
	settings.Registries[DefaultRegistryAlias].Repositories[1] = "changed"
	require.Equal(t, "oci://example.com/two", result[DefaultRegistryAlias].Repositories[1])
	delete(result, DefaultRegistryAlias)
	require.Len(t, settings.Registries, 1)
}
