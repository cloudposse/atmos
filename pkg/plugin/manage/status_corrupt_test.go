package manage_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/install"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestListAndInfoValidateConfigurationLikeOrdinaryCommands(t *testing.T) {
	cases := map[string]func(*schema.AtmosConfiguration){
		"package scope": func(c *schema.AtmosConfiguration) {
			c.Plugins.Packages = map[string]schema.PluginPackage{"a": {Source: "oci://ghcr.io/acme/a", Version: "1", Scope: "global"}}
		},
		"policy cache_ttl": func(c *schema.AtmosConfiguration) { c.Plugins.Policy.CacheTTL = "1h" },
		"max concurrency":  func(c *schema.AtmosConfiguration) { c.Plugins.MaxConcurrency = -1 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			config := &schema.AtmosConfiguration{}
			mutate(config)
			project := t.TempDir()
			_, err := manage.List(config, project)
			require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
			_, err = manage.Info(config, project, "a")
			require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
		})
	}
	// Negative: the same call with valid configuration lists normally.
	statuses, err := manage.List(&schema.AtmosConfiguration{}, t.TempDir())
	require.NoError(t, err)
	require.Empty(t, statuses)
}

func TestListReportsCorruptReleaseDistinctFromMissing(t *testing.T) {
	m, source, _ := setup(t)
	require.NoError(t, m.Add(t.Context(), manage.AddOptions{Alias: "installed", Source: source}))
	reloadConfiguration(t, m)
	m.Config.Plugins.CacheDir = m.Installer.Cache.Directory
	m.Config.Plugins.Packages["absent"] = schema.PluginPackage{Source: "oci://registry.example.com/acme/absent", Version: "^1"}

	statuses, err := manage.List(m.Config, m.Project)
	require.NoError(t, err)
	require.Equal(t, "installed", statuses[1].State, "an intact release is not reported corrupt")
	require.Empty(t, statuses[1].Hint)

	locked, err := pluginlock.Load(m.Project)
	require.NoError(t, err)
	entry := locked.Spec.Packages["installed"]
	payload, err := m.Installer.Cache.Installed(&entry, install.CurrentPlatform())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(payload, "planted"), []byte("x"), 0o644))

	statuses, err = manage.List(m.Config, m.Project)
	require.NoError(t, err)
	require.Equal(t, "missing", statuses[0].State)
	require.Equal(t, "corrupt", statuses[1].State)
	require.Empty(t, statuses[1].Path, "corrupt bytes are never offered as an executable path")
	require.Contains(t, statuses[1].Hint, "installed")
	require.Contains(t, statuses[1].Hint, filepath.Dir(payload))
	require.Contains(t, statuses[1].Hint, "atmos plugin install")

	info, err := manage.Info(m.Config, m.Project, "installed")
	require.NoError(t, err)
	require.Equal(t, "corrupt", info.State)
	require.Nil(t, info.Manifest)
	require.NotEmpty(t, info.Hint)
}

// corruptInstalledPackage installs a package, then plants a file in its cached payload.
func corruptInstalledPackage(t *testing.T) (m *manage.Manager, payload string) {
	t.Helper()
	m, source, _ := setup(t)
	require.NoError(t, m.Add(t.Context(), manage.AddOptions{Alias: "installed", Source: source}))
	reloadConfiguration(t, m)
	locked, err := pluginlock.Load(m.Project)
	require.NoError(t, err)
	entry := locked.Spec.Packages["installed"]
	payload, err = m.Installer.Cache.Installed(&entry, install.CurrentPlatform())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(payload, "planted"), []byte("x"), 0o644))
	return m, payload
}

func TestRestoreRepairsCorruptReleaseButFrozenRestoreNeverMutates(t *testing.T) {
	m, payload := corruptInstalledPackage(t)
	lockBefore, err := os.ReadFile(pluginlock.Path(m.Project))
	require.NoError(t, err)

	err = m.Restore(t.Context(), manage.RestoreOptions{Frozen: true})
	require.ErrorIs(t, err, errUtils.ErrPluginCacheCorrupt)
	require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)
	hints := strings.Join(errUtils.AllHints(err), "\n")
	require.Contains(t, hints, "installed", "the hint names the alias")
	require.Contains(t, hints, filepath.Dir(payload), "the hint names the cache path")
	require.Contains(t, hints, "atmos plugin install")
	require.FileExists(t, filepath.Join(payload, "planted"), "--frozen leaves the corrupt release untouched")
	matches, globErr := filepath.Glob(filepath.Dir(payload) + ".corrupt-*")
	require.NoError(t, globErr)
	require.Empty(t, matches)

	require.NoError(t, m.Restore(t.Context(), manage.RestoreOptions{}))
	require.NoFileExists(t, filepath.Join(payload, "planted"))
	matches, globErr = filepath.Glob(filepath.Dir(payload) + ".corrupt-*")
	require.NoError(t, globErr)
	require.Len(t, matches, 1, "the corrupt release is quarantined, not deleted")
	lockAfter, err := os.ReadFile(pluginlock.Path(m.Project))
	require.NoError(t, err)
	require.Equal(t, string(lockBefore), string(lockAfter), "repair re-fetches the locked digest and keeps the lock unchanged")
	require.NoError(t, m.Restore(t.Context(), manage.RestoreOptions{Frozen: true}), "the repaired cache satisfies --frozen")
}

func TestUpdateRepairsCorruptReleaseInPlace(t *testing.T) {
	m, payload := corruptInstalledPackage(t)
	require.NoError(t, m.Restore(t.Context(), manage.RestoreOptions{Update: true, Alias: "installed"}))
	require.NoFileExists(t, filepath.Join(payload, "planted"))
}

func TestIntactReleaseIsNeverQuarantinedByRestore(t *testing.T) {
	m, source, _ := setup(t)
	require.NoError(t, m.Add(t.Context(), manage.AddOptions{Alias: "installed", Source: source}))
	reloadConfiguration(t, m)
	require.NoError(t, m.Restore(t.Context(), manage.RestoreOptions{}))
	require.NoError(t, m.Restore(t.Context(), manage.RestoreOptions{Frozen: true}))
	matches, err := filepath.Glob(filepath.Join(m.Installer.Cache.Directory, "releases", "*", "*", "*.corrupt-*"))
	require.NoError(t, err)
	require.Empty(t, matches)
}
