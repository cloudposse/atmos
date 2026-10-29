package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/install"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
)

func TestSelectionSurfacesCorruptionNeverRunsOrSkipsIt(t *testing.T) {
	atmos, _ := automaticFixture(t)
	state, err := selectedPackages(t.Context(), atmos)
	require.NoError(t, err, "the first command installs the package")
	payload := state.Packages["fixture"].Path
	cache, err := install.NewCache(atmos.Plugins.CacheDir)
	require.NoError(t, err)
	file, err := pluginlock.Load(ProjectDirectory(atmos))
	require.NoError(t, err)
	declaration := atmos.Plugins.Packages["fixture"]

	// Negative: an intact release is selected without error.
	_, err = lockedSelection("fixture", &declaration, file, cache)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(payload, "planted"), []byte("x"), 0o600))
	_, err = lockedSelection("fixture", &declaration, file, cache)
	require.ErrorIs(t, err, errUtils.ErrPluginCacheCorrupt)
	require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)
	require.NotErrorIs(t, err, errUtils.ErrPluginNotInstalled)
	require.NotErrorIs(t, err, os.ErrNotExist, "available-auth selection skips not-exist errors; corruption must not be skipped")
	hints := strings.Join(errUtils.AllHints(err), "\n")
	require.Contains(t, hints, "fixture")
	require.Contains(t, hints, filepath.Dir(payload))
	require.Contains(t, hints, "atmos plugin install")

	// A deleted file is corruption as well, even though the OS reports it as absent.
	require.NoError(t, os.Remove(filepath.Join(payload, "planted")))
	require.NoError(t, os.Remove(filepath.Join(payload, "plugin.yaml")))
	_, err = lockedSelection("fixture", &declaration, file, cache)
	require.ErrorIs(t, err, errUtils.ErrPluginCacheCorrupt)
	require.NotErrorIs(t, err, os.ErrNotExist)
}

func TestOfflineOrdinaryCommandRefusesCorruptCache(t *testing.T) {
	atmos, requests := automaticFixture(t)
	state, err := selectedPackages(t.Context(), atmos)
	require.NoError(t, err)
	payload := state.Packages["fixture"].Path
	require.NoError(t, os.WriteFile(filepath.Join(payload, "planted"), []byte("x"), 0o600))
	atmos.Plugins.Policy.Offline = true
	requests.Store(0)

	_, err = selectedPackages(t.Context(), atmos)
	require.ErrorIs(t, err, errUtils.ErrPluginCacheCorrupt)
	require.NotErrorIs(t, err, errUtils.ErrPluginNotInstalled)
	require.Zero(t, requests.Load(), "offline mode never contacts the registry")
	require.FileExists(t, filepath.Join(payload, "planted"), "an offline command never mutates the cache")
	matches, globErr := filepath.Glob(filepath.Dir(payload) + ".corrupt-*")
	require.NoError(t, globErr)
	require.Empty(t, matches)
}
