package manage_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDefaultRegistryRemovalAndExplicitReaddition(t *testing.T) {
	for _, override := range []bool{false, true} {
		t.Run(map[bool]string{false: "built-in", true: "explicit override"}[override], func(t *testing.T) {
			m, _, _ := setup(t)
			if override {
				require.NoError(t, m.EditRegistry(t.Context(), config.DefaultRegistryAlias, &schema.PluginRegistry{Catalog: "oci://example.com/private/catalog:stable"}))
				reloadConfiguration(t, m)
			}
			require.NoError(t, m.EditRegistry(t.Context(), config.DefaultRegistryAlias, nil))
			reloadConfiguration(t, m)
			require.NotNil(t, m.Config.Plugins.Discovery.DefaultRegistry)
			require.False(t, *m.Config.Plugins.Discovery.DefaultRegistry)
			require.NotContains(t, m.Config.Plugins.Registries, config.DefaultRegistryAlias)
			require.Empty(t, config.EffectiveRegistries(&m.Config.Plugins))
			require.Contains(t, string(readConfig(t, m)), "# keep this comment")
			require.ErrorIs(t, m.EditRegistry(t.Context(), config.DefaultRegistryAlias, nil), errUtils.ErrPluginRegistryNotFound)

			// Explicit configuration remains usable even when the implicit default is disabled.
			registry := schema.PluginRegistry{Catalog: "oci://example.com/replacement/catalog:stable"}
			require.NoError(t, m.EditRegistry(t.Context(), config.DefaultRegistryAlias, &registry))
			reloadConfiguration(t, m)
			require.False(t, *m.Config.Plugins.Discovery.DefaultRegistry)
			reference, err := manage.ResolveCatalogReference(m.Config, config.DefaultRegistryAlias)
			require.NoError(t, err)
			require.Equal(t, registry.Catalog, reference)
			require.NoError(t, m.EditRegistry(t.Context(), config.DefaultRegistryAlias, nil))
			reloadConfiguration(t, m)
			require.Empty(t, config.EffectiveRegistries(&m.Config.Plugins))
		})
	}
}

func TestDefaultRegistryLookupAndWholeEntryOverride(t *testing.T) {
	atmos := &schema.AtmosConfiguration{}
	host, err := manage.ResolveRegistryHost(atmos, config.DefaultRegistryAlias)
	require.NoError(t, err)
	require.Equal(t, "ghcr.io", host)
	reference, err := manage.ResolveCatalogReference(atmos, config.DefaultRegistryAlias)
	require.NoError(t, err)
	require.Equal(t, config.DefaultCatalog, reference)
	atmos.Plugins.Registries = map[string]schema.PluginRegistry{config.DefaultRegistryAlias: {Host: "private.example.com"}}
	host, err = manage.ResolveRegistryHost(atmos, config.DefaultRegistryAlias)
	require.NoError(t, err)
	require.Equal(t, "private.example.com", host)
	_, err = manage.ResolveCatalogReference(atmos, config.DefaultRegistryAlias)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
}

func TestDefaultRegistryRemovalRejectsMalformedConfiguration(t *testing.T) {
	m, _, _ := setup(t)
	before := []byte("plugins: [invalid YAML")
	path := filepath.Join(m.Project, m.ConfigFile)
	require.NoError(t, os.WriteFile(path, before, 0o600))
	require.Error(t, m.EditRegistry(t.Context(), config.DefaultRegistryAlias, nil))
	require.Equal(t, before, readConfig(t, m), "failed removal must not partially edit configuration")
}
