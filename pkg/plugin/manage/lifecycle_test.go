package manage_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	"github.com/cloudposse/atmos/pkg/schema"
	edit "github.com/cloudposse/atmos/pkg/yaml"
)

func reloadConfiguration(t *testing.T, m *manage.Manager) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(m.Project, m.ConfigFile))
	require.NoError(t, err)
	*m.Config = schema.AtmosConfiguration{}
	require.NoError(t, yaml.Unmarshal(raw, m.Config))
}

func readConfig(t *testing.T, m *manage.Manager) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(m.Project, m.ConfigFile))
	require.NoError(t, err)
	return raw
}

func TestLifecyclePreservesDeclarationAndRemovesRelease(t *testing.T) {
	m, source, _ := setup(t)
	require.NoError(t, m.Add(t.Context(), manage.AddOptions{Source: source}))
	reloadConfiguration(t, m)
	require.Contains(t, m.Config.Plugins.Packages, "example-fixture")
	// Settings the user wrote beside source and version must survive a version change.
	configured, err := edit.SetJSON(readConfig(t, m), "plugins.packages.example-fixture.config", []byte(`{"issuer":"https://issuer.example.com"}`))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(m.Project, m.ConfigFile), configured, 0o644))
	reloadConfiguration(t, m)
	require.NoError(t, m.Add(t.Context(), manage.AddOptions{Alias: "example-fixture", Source: source, Constraint: "^1"}))
	reloadConfiguration(t, m)
	require.Equal(t, map[string]any{"issuer": "https://issuer.example.com"}, m.Config.Plugins.Packages["example-fixture"].Config)
	require.NoError(t, m.Restore(t.Context(), manage.RestoreOptions{Update: true, Alias: "example-fixture"}))
	require.ErrorIs(t, m.Restore(t.Context(), manage.RestoreOptions{Alias: "missing"}), errUtils.ErrPluginNotInstalled)
	require.ErrorIs(t, m.Add(t.Context(), manage.AddOptions{Alias: "invalid alias", Source: source}), errUtils.ErrPluginInvalidConfig)
	require.ErrorIs(t, m.Add(t.Context(), manage.AddOptions{Alias: "duplicate", Source: source}), errUtils.ErrDuplicatePluginExtension)
	require.ErrorIs(t, m.Remove(t.Context(), "invalid alias"), errUtils.ErrPluginInvalidConfig)
	require.ErrorIs(t, m.Remove(t.Context(), "missing"), errUtils.ErrPluginNotInstalled)
	require.NoError(t, m.Remove(t.Context(), "example-fixture"))
	reloadConfiguration(t, m)
	require.Empty(t, m.Config.Plugins.Packages)
	locked, err := pluginlock.Load(m.Project)
	require.NoError(t, err)
	require.Empty(t, locked.Spec.Packages)
}

func TestRemovalProtectsAuthReferencesAndPathPackages(t *testing.T) {
	m, source, directory := setup(t)
	require.NoError(t, m.Add(t.Context(), manage.AddOptions{Alias: "fixture", Source: source}))
	reloadConfiguration(t, m)
	locked, err := pluginlock.Load(m.Project)
	require.NoError(t, err)
	entry := locked.Spec.Packages["fixture"]
	entry.Exports = []pluginlock.Export{{Capability: "auth.provider", Name: "acme/oidc"}, {Capability: "auth.identity", Name: "acme/exchange"}}
	locked.Spec.Packages["fixture"] = entry
	raw, err := yaml.Marshal(locked)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(pluginlock.Path(m.Project), raw, 0o644))
	m.Config.Auth.Providers = map[string]schema.Provider{"custom": {Kind: "acme/oidc"}}
	require.ErrorIs(t, m.Remove(t.Context(), "fixture"), errUtils.ErrPluginInUse)
	m.Config.Auth.Providers = nil
	m.Config.Auth.Identities = map[string]schema.Identity{"role": {Kind: "acme/exchange"}}
	require.ErrorIs(t, m.Remove(t.Context(), "fixture"), errUtils.ErrPluginInUse)
	m.Config.Auth.Identities = nil
	// A profile adds path beside the declared release: the path comes from configuration, so removal
	// explains where to edit it instead of rewriting a file it did not create.
	m.Config.Plugins.Packages["fixture"] = schema.PluginPackage{Source: source, Version: "^1", Path: directory}
	m.Config.ActiveProfiles = []string{"sandbox"}
	err = m.Remove(t.Context(), "fixture")
	require.ErrorIs(t, err, errUtils.ErrPluginPathPackage)
	hints := strings.Join(errUtils.AllHints(err), "\n")
	require.Contains(t, hints, "plugins.packages.fixture.path")
	require.Contains(t, hints, "sandbox")
	_, statErr := os.Stat(pluginlock.Path(m.Project))
	require.NoError(t, statErr, "the lock entry stays")

	// Frozen installs refuse while the active configuration runs a local build.
	err = m.Restore(t.Context(), manage.RestoreOptions{Frozen: true})
	require.ErrorIs(t, err, errUtils.ErrPluginFrozenDrift)
	hints = strings.Join(errUtils.AllHints(err), "\n")
	require.Contains(t, hints, "active configuration sets `path`")
	require.Contains(t, hints, "sandbox")
	require.NotContains(t, hints, "dev")

	// Negative: without the path the package is a release again and removal works as before.
	m.Config.Plugins.Packages["fixture"] = schema.PluginPackage{Source: source, Version: "^1"}
	require.NoError(t, m.Remove(t.Context(), "fixture"))
}

func TestRegistryEditsPreserveUnrelatedConfiguration(t *testing.T) {
	m, _, _ := setup(t)
	registry := &schema.PluginRegistry{Catalog: "oci://ghcr.io/acme/catalog:stable"}
	require.NoError(t, m.EditRegistry(t.Context(), "acme", registry))
	reloadConfiguration(t, m)
	require.Equal(t, *registry, m.Config.Plugins.Registries["acme"])
	require.NoError(t, m.EditRegistry(t.Context(), "acme", nil))
	reloadConfiguration(t, m)
	require.Empty(t, m.Config.Plugins.Registries)
	require.ErrorIs(t, m.EditRegistry(t.Context(), "missing", nil), errUtils.ErrPluginRegistryNotFound)
	require.ErrorIs(t, m.EditRegistry(t.Context(), "bad alias", registry), errUtils.ErrPluginInvalidConfig)
	require.ErrorIs(t, m.EditRegistry(t.Context(), "acme", &schema.PluginRegistry{Catalog: "https://example.com"}), errUtils.ErrPluginInvalidConfig)
	require.Error(t, m.EditRegistry(t.Context(), "acme", &schema.PluginRegistry{Catalog: "oci://bad reference"}))
	raw, err := os.ReadFile(filepath.Join(m.Project, m.ConfigFile))
	require.NoError(t, err)
	require.Contains(t, string(raw), "# keep this comment")
}

func TestStatusIncludesMissingDisabledInstalledAndPath(t *testing.T) {
	m, source, directory := setup(t)
	require.NoError(t, m.Add(t.Context(), manage.AddOptions{Alias: "installed", Source: source}))
	reloadConfiguration(t, m)
	m.Config.Plugins.CacheDir = m.Installer.Cache.Directory
	disabled := false
	m.Config.Plugins.Packages["disabled"] = schema.PluginPackage{Enabled: &disabled}
	m.Config.Plugins.Packages["missing"] = schema.PluginPackage{Source: "oci://registry.example.com/acme/missing", Version: "^1"}
	m.Config.Plugins.Packages["local"] = schema.PluginPackage{Path: directory}
	statuses, err := manage.List(m.Config, m.Project)
	require.NoError(t, err)
	require.Equal(t, []string{"disabled", "installed", "local", "missing"}, []string{statuses[0].Alias, statuses[1].Alias, statuses[2].Alias, statuses[3].Alias})
	require.Equal(t, []string{"disabled", "installed", "path", "missing"}, []string{statuses[0].State, statuses[1].State, statuses[2].State, statuses[3].State})
	local := statuses[2]
	require.Equal(t, "path", local.Source)
	require.Equal(t, directory, local.Path)
	require.Equal(t, "example/fixture", local.Plugin, "name and version come from the local manifest")
	require.NotEmpty(t, local.Version)
	require.Empty(t, statuses[1].Source, "a release has no path source")
	info, err := manage.Info(m.Config, m.Project, "installed")
	require.NoError(t, err)
	require.Equal(t, "example/fixture", info.Manifest.Metadata.Name)
	_, err = manage.Info(m.Config, m.Project, "absent")
	require.ErrorIs(t, err, errUtils.ErrPluginNotInstalled)
	info, err = manage.Info(m.Config, m.Project, "missing")
	require.NoError(t, err)
	require.Nil(t, info.Manifest)
	// An unreadable local manifest is reported as broken with a repair hint, never as the release.
	require.NoError(t, os.Remove(filepath.Join(directory, "plugin.yaml")))
	info, err = manage.Info(m.Config, m.Project, "local")
	require.NoError(t, err)
	require.Equal(t, "broken", info.State)
	require.Contains(t, info.Hint, "plugins.packages.local.path")
	require.Nil(t, info.Manifest)
	// Disabled local declarations are excluded from path selection, so inspecting
	// one must not attempt to read its missing manifest.
	m.Config.Plugins.Packages["local"] = schema.PluginPackage{Path: directory, Enabled: &disabled}
	info, err = manage.Info(m.Config, m.Project, "local")
	require.NoError(t, err)
	require.Equal(t, "disabled", info.State)
	require.Empty(t, info.Path)
	require.Empty(t, info.Hint)
	require.Nil(t, info.Manifest)
	m.Config.Plugins.Packages["bad"] = schema.PluginPackage{Path: "  "}
	_, err = manage.List(m.Config, m.Project)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
}

var (
	_ = schema.Provider{Kind: ""}
	_ = schema.Identity{Kind: ""}
)
