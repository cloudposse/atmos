package manage_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/ui"
)

// publishVersions publishes the fixture package to a new repository, once per version, and
// returns the OCI source. The fixture directory is rewritten for each version.
func publishVersions(t *testing.T, manager *manage.Manager, source, directory, repository string, versions ...string) string {
	t.Helper()
	original, err := os.ReadFile(filepath.Join(directory, "plugin.yaml"))
	require.NoError(t, err)
	base := strings.TrimSuffix(strings.TrimPrefix(source, "oci://"), "/acme/plugin")
	target := base + "/acme/" + repository
	parsed, err := name.NewRepository(target)
	require.NoError(t, err)
	for _, version := range versions {
		rewritten := strings.ReplaceAll(string(original), "version: 1.0.0", "version: "+version)
		require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(rewritten), 0o644))
		index, err := pluginoci.BuildIndex(directory, manager.Installer.Descriptors)
		require.NoError(t, err)
		require.NoError(t, manager.Installer.Registry.WriteIndex(t.Context(), parsed.Tag(version), index))
	}
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), original, 0o644))
	return "oci://" + target
}

func TestAddWithoutVersionPinsTheInstalledRelease(t *testing.T) {
	cases := []struct{ name, version, want string }{
		{"stable one-x pins the major", "1.4.2", "^1.4.2"},
		{"zero-x pins the minor via caret semantics", "0.1.0", "^0.1.0"},
		{"pre-release is an exact pin", "1.0.0-rc.1", "1.0.0-rc.1"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			manager, source, directory := setup(t)
			target := publishVersions(t, manager, source, directory, "pinned", testCase.version)
			require.NoError(t, manager.Add(t.Context(), manage.AddOptions{Alias: "pinned", Source: target}))
			reloadConfiguration(t, manager)
			assert.Equal(t, testCase.want, manager.Config.Plugins.Packages["pinned"].Version)
			locked, err := pluginlock.Load(manager.Project)
			require.NoError(t, err)
			entry := locked.Spec.Packages["pinned"]
			assert.Equal(t, testCase.want, entry.Constraint)
			assert.Equal(t, testCase.version, entry.Version)
		})
	}
}

func TestExplicitVersionIsStoredVerbatim(t *testing.T) {
	manager, source, _ := setup(t)
	require.NoError(t, manager.Add(t.Context(), manage.AddOptions{Alias: "fixture", Source: source, Constraint: "~1.0"}))
	reloadConfiguration(t, manager)
	assert.Equal(t, "~1.0", manager.Config.Plugins.Packages["fixture"].Version)
	locked, err := pluginlock.Load(manager.Project)
	require.NoError(t, err)
	assert.Equal(t, "~1.0", locked.Spec.Packages["fixture"].Constraint)
}

func TestRerunWithoutVersionKeepsDeclaredConstraintAndIsIdempotent(t *testing.T) {
	manager, source, directory := setup(t)
	ioContext, err := iolib.NewContext()
	require.NoError(t, err)
	ui.InitFormatter(ioContext)
	t.Cleanup(ui.Reset)
	target := publishVersions(t, manager, source, directory, "again", "1.0.0")
	require.NoError(t, manager.Add(t.Context(), manage.AddOptions{Alias: "again", Source: target, Constraint: "^1"}))
	reloadConfiguration(t, manager)
	configPath := filepath.Join(manager.Project, manager.ConfigFile)
	configBefore, err := os.ReadFile(configPath)
	require.NoError(t, err)
	lockBefore, err := os.ReadFile(pluginlock.Path(manager.Project))
	require.NoError(t, err)
	var output bytes.Buffer
	t.Cleanup(iolib.PushUIWriter(&output))

	// No alias and no version: the alias is derived from the source before the previous entry is looked up.
	require.NoError(t, manager.Add(t.Context(), manage.AddOptions{Source: target}))

	configAfter, err := os.ReadFile(configPath)
	require.NoError(t, err)
	lockAfter, err := os.ReadFile(pluginlock.Path(manager.Project))
	require.NoError(t, err)
	assert.Equal(t, string(configBefore), string(configAfter), "an unchanged install must not rewrite the declaration")
	assert.Equal(t, string(lockBefore), string(lockAfter), "an unchanged install must not rewrite the lock")
	assert.Contains(t, ansi.Strip(output.String()), "already up to date")
	reloadConfiguration(t, manager)
	assert.Equal(t, "^1", manager.Config.Plugins.Packages["again"].Version)
	assert.NotContains(t, manager.Config.Plugins.Packages, "example-fixture", "the existing alias must be reused")
}

func TestRerunDoesNotAdvanceBeyondTheLockedRelease(t *testing.T) {
	manager, source, directory := setup(t)
	target := publishVersions(t, manager, source, directory, "stay", "1.0.0")
	require.NoError(t, manager.Add(t.Context(), manage.AddOptions{Alias: "stay", Source: target, Constraint: "^1"}))
	reloadConfiguration(t, manager)
	publishVersions(t, manager, source, directory, "stay", "1.1.0")
	require.NoError(t, manager.Add(t.Context(), manage.AddOptions{Alias: "stay", Source: target}))
	locked, err := pluginlock.Load(manager.Project)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", locked.Spec.Packages["stay"].Version, "install must never advance; update does")
}

func TestDerivedAliasCannotReplaceAnotherSource(t *testing.T) {
	for _, owner := range []string{"declaration and lock", "declaration only", "lock only"} {
		t.Run(owner, func(t *testing.T) {
			manager, source, directory := setup(t)
			require.NoError(t, manager.Add(t.Context(), manage.AddOptions{Alias: "example-fixture", Source: source}))
			reloadConfiguration(t, manager)
			other := publishVersions(t, manager, source, directory, "mirror", "1.0.0")
			switch owner {
			case "declaration only":
				require.NoError(t, pluginlock.Save(manager.Project, pluginlock.New()))
			case "lock only":
				clear(manager.Config.Plugins.Packages)
				require.NoError(t, os.WriteFile(filepath.Join(manager.Project, manager.ConfigFile), []byte("# retained config\nunrelated: preserved\n"), 0o644))
			}
			configBefore, err := os.ReadFile(filepath.Join(manager.Project, manager.ConfigFile))
			require.NoError(t, err)
			lockBefore, err := os.ReadFile(pluginlock.Path(manager.Project))
			require.NoError(t, err)
			err = manager.Add(t.Context(), manage.AddOptions{Source: other})
			require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
			require.Contains(t, strings.Join(errUtils.AllHints(err), "\n"), "--as")
			configAfter, err := os.ReadFile(filepath.Join(manager.Project, manager.ConfigFile))
			require.NoError(t, err)
			lockAfter, err := os.ReadFile(pluginlock.Path(manager.Project))
			require.NoError(t, err)
			require.Equal(t, configBefore, configAfter, "implicit alias collision must preserve the existing declaration")
			require.Equal(t, lockBefore, lockAfter, "implicit alias collision must preserve the existing lock")
		})
	}
}

func TestExplicitAliasMayReplaceAnotherSource(t *testing.T) {
	manager, source, directory := setup(t)
	require.NoError(t, manager.Add(t.Context(), manage.AddOptions{Alias: "example-fixture", Source: source}))
	reloadConfiguration(t, manager)
	other := publishVersions(t, manager, source, directory, "mirror", "1.0.0")
	require.NoError(t, manager.Add(t.Context(), manage.AddOptions{Alias: "example-fixture", Source: other}))
	reloadConfiguration(t, manager)
	require.Equal(t, other, manager.Config.Plugins.Packages["example-fixture"].Source)
	locked, err := pluginlock.Load(manager.Project)
	require.NoError(t, err)
	require.Equal(t, other, locked.Spec.Packages["example-fixture"].Source)
}
