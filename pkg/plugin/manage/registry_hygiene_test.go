package manage_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/discovery"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRemovingUnknownRegistryListsConfiguredNames(t *testing.T) {
	m, _, _ := setup(t)
	for _, alias := range []string{"zulu", "alpha"} {
		require.NoError(t, m.EditRegistry(t.Context(), alias, &schema.PluginRegistry{Catalog: "oci://ghcr.io/acme/" + alias + ":stable"}))
		reloadConfiguration(t, m)
	}
	err := m.EditRegistry(t.Context(), "missing", nil)
	require.ErrorIs(t, err, errUtils.ErrPluginRegistryNotFound)
	require.NotErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	hints := strings.Join(errUtils.AllHints(err), " ")
	assert.Contains(t, hints, "alpha, cloudposse, zulu", "configured names are sorted")
	assert.Contains(t, strings.Join(errUtils.AllDetails(err), " "), `"missing"`)
}

func TestRemovingUnknownRegistryWithNoneConfiguredExplainsHowToAddOne(t *testing.T) {
	m, _, _ := setup(t)
	m.Config.Plugins.Discovery.DefaultRegistry = new(false)
	err := m.EditRegistry(t.Context(), "missing", nil)
	require.ErrorIs(t, err, errUtils.ErrPluginRegistryNotFound)
	assert.Contains(t, strings.Join(errUtils.AllHints(err), " "), "atmos plugin registry add")
}

func TestInvalidRegistryAliasIsExplained(t *testing.T) {
	m, _, _ := setup(t)
	err := m.EditRegistry(t.Context(), "Bad Alias", &schema.PluginRegistry{Catalog: "oci://ghcr.io/acme/catalog:stable"})
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	assert.NotEmpty(t, errUtils.AllHints(err))
}

func TestResolveRegistryHostDistinguishesAliasesHostsAndTypos(t *testing.T) {
	config := &schema.AtmosConfiguration{Plugins: schema.PluginsConfig{Registries: map[string]schema.PluginRegistry{
		"corporate": {Host: "registry.example.com"},
	}}}
	host, err := manage.ResolveRegistryHost(config, "corporate")
	require.NoError(t, err)
	assert.Equal(t, "registry.example.com", host)
	host, err = manage.ResolveRegistryHost(config, "ghcr.io")
	require.NoError(t, err)
	assert.Equal(t, "ghcr.io", host)
	host, err = manage.ResolveRegistryHost(config, "localhost:5000")
	require.NoError(t, err)
	assert.Equal(t, "localhost:5000", host)

	// A bare word that is neither a configured alias nor shaped like a host is a mistyped alias.
	_, err = manage.ResolveRegistryHost(config, "corporat")
	require.ErrorIs(t, err, errUtils.ErrPluginRegistryNotFound)
	assert.Contains(t, strings.Join(errUtils.AllHints(err), " "), "corporate")

	_, err = manage.ResolveRegistryHost(nil, "ghcr.io")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
}

func TestResolveCatalogReferenceAcceptsAliasesAndOCIAndExplainsEverythingElse(t *testing.T) {
	config := &schema.AtmosConfiguration{Plugins: schema.PluginsConfig{Registries: map[string]schema.PluginRegistry{
		"corporate": {Catalog: "oci://registry.example.com/plugins/catalog:stable"},
		"hostonly":  {Host: "registry.example.com"},
		"another":   {Catalog: "oci://registry.example.com/other/catalog:stable"},
	}}}
	reference, err := manage.ResolveCatalogReference(config, "corporate")
	require.NoError(t, err)
	assert.Equal(t, "oci://registry.example.com/plugins/catalog:stable", reference)
	reference, err = manage.ResolveCatalogReference(config, "oci://ghcr.io/acme/catalog:v1")
	require.NoError(t, err)
	assert.Equal(t, "oci://ghcr.io/acme/catalog:v1", reference)

	_, err = manage.ResolveCatalogReference(config, "hostonly")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	assert.Contains(t, strings.Join(errUtils.AllHints(err), " "), "oci://")

	_, err = manage.ResolveCatalogReference(config, "ghcr.io/acme/catalog:v1")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	hints := strings.Join(errUtils.AllHints(err), " ")
	assert.Contains(t, hints, "prefix with `oci://`")
	assert.Contains(t, hints, "another, cloudposse, corporate", "only aliases that configure a catalog are offered, sorted")
	assert.NotContains(t, hints, "hostonly")

	_, err = manage.ResolveCatalogReference(&schema.AtmosConfiguration{Plugins: schema.PluginsConfig{Discovery: schema.PluginDiscoveryConfig{DefaultRegistry: new(false)}}}, "missing")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	assert.Contains(t, strings.Join(errUtils.AllHints(err), " "), "atmos plugin registry add")

	_, err = manage.ResolveCatalogReference(nil, "corporate")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
}

const commentedRegistryConfig = `# Top comment: keep me
base_path: "."   # inline comment on base_path

# Plugins section comment
plugins:
  registries:
    # comment about corporate
    corporate:
      catalog: oci://registry.example.com/old/catalog:stable   # inline catalog comment
      identity: reader
    # comment about other
    other:
      host: ghcr.io

# Trailing comment: keep me
`

func TestRegistryEditsPreserveCommentsBlankLinesAndBlockStyle(t *testing.T) {
	m, _, _ := setup(t)
	path := filepath.Join(m.Project, m.ConfigFile)
	require.NoError(t, os.WriteFile(path, []byte(commentedRegistryConfig), 0o644))
	reloadConfiguration(t, m)

	// Replacing an entry rewrites its own keys and drops the ones no longer set.
	require.NoError(t, m.EditRegistry(t.Context(), "corporate", &schema.PluginRegistry{
		Catalog: "oci://registry.example.com/new/catalog:stable", Repositories: []string{"oci://registry.example.com/acme/one", "oci://registry.example.com/acme/two"},
	}))
	updated, err := os.ReadFile(path)
	require.NoError(t, err)
	text := string(updated)
	for _, fragment := range []string{
		"# Top comment: keep me", "   # inline comment on base_path", "\n\n# Plugins section comment\nplugins:",
		"    # comment about corporate\n    corporate:", "    # comment about other\n    other:\n      host: ghcr.io\n", "\n\n# Trailing comment: keep me\n",
		"catalog: oci://registry.example.com/new/catalog:stable", "- oci://registry.example.com/acme/one", "- oci://registry.example.com/acme/two",
	} {
		assert.Contains(t, text, fragment)
	}
	assert.NotContains(t, text, "identity: reader", "keys absent from the new entry are removed")
	assert.NotContains(t, text, "old/catalog")
	assert.NotContains(t, text, `{"`, "entries stay in block style")
	reloadConfiguration(t, m)
	assert.Equal(t, []string{"oci://registry.example.com/acme/one", "oci://registry.example.com/acme/two"}, m.Config.Plugins.Registries["corporate"].Repositories)
	assert.Empty(t, m.Config.Plugins.Registries["corporate"].Identity)

	// A new alias is added beside the others in block style.
	require.NoError(t, m.EditRegistry(t.Context(), "third", &schema.PluginRegistry{Host: "registry.example.com", Auth: "docker"}))
	added, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(added), "# comment about other")
	assert.Contains(t, string(added), "third:")
	assert.NotContains(t, string(added), `{"`)

	// Removal deletes the entry and its head comment and leaves the rest untouched.
	reloadConfiguration(t, m)
	require.NoError(t, m.EditRegistry(t.Context(), "other", nil))
	removed, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(removed), "comment about other")
	for _, fragment := range []string{"# Top comment: keep me", "# comment about corporate", "# Trailing comment: keep me"} {
		assert.Contains(t, string(removed), fragment)
	}
	reloadConfiguration(t, m)
	assert.NotContains(t, m.Config.Plugins.Registries, "other")
	assert.Contains(t, m.Config.Plugins.Registries, "third")
}

func TestManagerSearchUsesInstallerRegistryAndReusesMetadataOffline(t *testing.T) {
	manager, source, _ := setup(t)
	manager.Config.Plugins.CacheDir = t.TempDir()
	manager.Config.Plugins.Registries = map[string]schema.PluginRegistry{"internal": {Repositories: []string{source}}}
	results, err := manager.Search(t.Context(), discovery.Request{Registry: "internal"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "internal", results[0].Registry)
	require.Equal(t, "example/fixture", results[0].Name)
	require.Equal(t, "1.0.0", results[0].Versions[0].Version)
	manager.Config.Plugins.Policy.Offline = true
	manager.Installer.Registry = nil // Offline search must never need a registry connection.
	cached, err := manager.Search(t.Context(), discovery.Request{Registry: "internal"})
	require.NoError(t, err)
	require.Equal(t, results, cached)
	_, err = manager.Search(t.Context(), discovery.Request{Registry: "internal", Refresh: true})
	require.ErrorIs(t, err, errUtils.ErrPluginOfflineArtifact)
}
