package bootstrap

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/plugin/install"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

func requireNoLock(t *testing.T, atmos *schema.AtmosConfiguration) {
	t.Helper()
	_, err := os.Stat(pluginlock.Path(ProjectDirectory(atmos)))
	require.ErrorIs(t, err, os.ErrNotExist, "the lock must not be written")
}

// withCI makes the CI decision deterministic. Callers must not run in parallel.
func withCI(t *testing.T, value bool) {
	t.Helper()
	previous := inCI
	inCI = func() bool { return value }
	t.Cleanup(func() { inCI = previous })
}

func captureNotices(t *testing.T) *bytes.Buffer {
	t.Helper()
	ioContext, err := iolib.NewContext()
	require.NoError(t, err)
	ui.InitFormatter(ioContext)
	t.Cleanup(ui.Reset)
	var output bytes.Buffer
	t.Cleanup(iolib.PushUIWriter(&output))
	return &output
}

func TestOrdinaryCommandWithoutPluginKindsNeverTouchesTheRegistry(t *testing.T) {
	atmos, requests := automaticFixture(t)
	cleanup, err := Prepare(t.Context(), atmos)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	require.Zero(t, requests.Load(), "an unlocked package that nothing references must not be downloaded")
	requireNoLock(t, atmos)
	assert.NoDirExists(t, filepath.Join(atmos.Plugins.CacheDir, "releases"))

	// Built-in kinds do not reference a plugin either.
	atmos.Auth.Providers = map[string]schema.Provider{"sso": {Kind: "aws/iam-identity-center"}}
	cleanup, err = Prepare(t.Context(), atmos)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	require.Zero(t, requests.Load())
	requireNoLock(t, atmos)
}

func TestReferencedKindInstallsUnlockedPackageAndAnnouncesIt(t *testing.T) {
	atmos, requests := automaticFixture(t)
	atmos.Auth.Providers = map[string]schema.Provider{"corp": {Kind: "test/unsatisfied-kind"}}
	output := captureNotices(t)

	cleanup, err := Prepare(t.Context(), atmos)
	require.NoError(t, err)
	t.Cleanup(cleanup)

	require.Positive(t, requests.Load())
	locked, err := pluginlock.Load(ProjectDirectory(atmos))
	require.NoError(t, err)
	entry := locked.Spec.Packages["fixture"]
	plain := ansi.Strip(output.String())
	assert.Equal(t, 1, strings.Count(plain, "installed automatically"), "exactly one notice per package: %s", plain)
	assert.Contains(t, plain, atmos.Plugins.Packages["fixture"].Source)
	assert.Contains(t, plain, entry.IndexDigest)
	assert.Contains(t, plain, "unverified")
	assert.NotContains(t, plain, "🔌")
}

func TestLockedExportSatisfiesKindWithoutNetworkAccess(t *testing.T) {
	atmos, requests := automaticFixture(t)
	cachedBootstrapAuth(t, atmos)
	requests.Store(0)
	cleanup, err := Prepare(t.Context(), atmos)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	require.Zero(t, requests.Load(), "a cached, locked export needs no registry, and the unrelated unlocked package is not fetched")
	locked, err := pluginlock.Load(ProjectDirectory(atmos))
	require.NoError(t, err)
	require.NotContains(t, locked.Spec.Packages, "fixture")
}

func TestUnlockedPackageIsRefusedInCI(t *testing.T) {
	withCI(t, true)
	atmos, requests := automaticFixture(t)
	atmos.Auth.Providers = map[string]schema.Provider{"corp": {Kind: "test/unsatisfied-kind"}}
	_, err := Prepare(t.Context(), atmos)
	require.ErrorIs(t, err, errUtils.ErrPluginUnlockedInCI)
	assert.Contains(t, strings.Join(errUtils.AllHints(err), "\n"), "atmos plugin install")
	assert.Contains(t, strings.Join(errUtils.AllHints(err), "\n"), ".atmos.plugins.lock.yaml")
	require.Zero(t, requests.Load(), "CI refuses before contacting the registry")
	requireNoLock(t, atmos)

	// Negative: the same configuration outside CI installs.
	withCI(t, false)
	cleanup, err := Prepare(t.Context(), atmos)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	require.Positive(t, requests.Load())
}

func TestCIRestoresLockedPackagesAndRefusesCorruptCache(t *testing.T) {
	atmos, requests := automaticFixture(t)
	state, err := selectedPackages(t.Context(), atmos)
	require.NoError(t, err)
	payload := state.Packages["fixture"].Path

	withCI(t, true)
	atmos.Plugins.CacheDir = t.TempDir()
	requests.Store(0)
	_, err = selectedPackages(t.Context(), atmos)
	require.NoError(t, err, "a locked package is reproducible, so CI may restore it")
	require.Positive(t, requests.Load())

	cache, err := install.NewCache(atmos.Plugins.CacheDir)
	require.NoError(t, err)
	file, err := pluginlock.Load(ProjectDirectory(atmos))
	require.NoError(t, err)
	entry := file.Spec.Packages["fixture"]
	directory, err := cache.Installed(&entry, install.CurrentPlatform())
	require.NoError(t, err)
	require.NotEqual(t, payload, directory)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "planted"), []byte("x"), 0o600))
	requests.Store(0)
	_, err = selectedPackages(t.Context(), atmos)
	require.ErrorIs(t, err, errUtils.ErrPluginCacheCorrupt)
	require.Zero(t, requests.Load())
	assert.Contains(t, strings.Join(errUtils.AllHints(err), "\n"), "fixture")
	assert.FileExists(t, filepath.Join(directory, "planted"), "CI never mutates the cache")
}

func TestDisabledPackagesAreNeverNeeded(t *testing.T) {
	atmos, requests := automaticFixture(t)
	off := false
	declaration := atmos.Plugins.Packages["fixture"]
	declaration.Enabled = &off
	atmos.Plugins.Packages["fixture"] = declaration
	atmos.Auth.Providers = map[string]schema.Provider{"corp": {Kind: "test/unsatisfied-kind"}}
	cleanup, err := Prepare(t.Context(), atmos)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	require.Zero(t, requests.Load())
	requireNoLock(t, atmos)
}

func TestInvalidPackageIsRejectedBeforeAnyNotice(t *testing.T) {
	atmos, requests := automaticFixture(t)
	atmos.Plugins.Packages["broken"] = schema.PluginPackage{Version: "^1"}
	output := captureNotices(t)
	_, err := Prepare(t.Context(), atmos)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	assert.Contains(t, err.Error(), "plugins.packages.broken.source")
	assert.Empty(t, output.String(), "validation runs before any notice")
	require.Zero(t, requests.Load())
}

func TestNeededLocalPackageErrorsAreNotHiddenByKindSelection(t *testing.T) {
	for _, invalid := range []string{"missing manifest", "unknown capability"} {
		t.Run(invalid, func(t *testing.T) {
			atmos, requests := automaticFixture(t)
			directory := localPackageFixture(t)
			declareLocal(atmos, "fixture", directory)
			atmos.Auth.Providers = map[string]schema.Provider{"corp": {Kind: "test/local-provider"}}
			if invalid == "missing manifest" {
				require.NoError(t, os.Remove(filepath.Join(directory, "plugin.yaml")))
			} else {
				raw, err := os.ReadFile(filepath.Join(directory, "plugin.yaml"))
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(strings.ReplaceAll(string(raw), "commands:", "unknown:")), 0o600))
			}
			cleanup, err := Prepare(t.Context(), atmos)
			require.Error(t, err, "unreadable local metadata must fail activation rather than hide a needed package")
			require.Nil(t, cleanup)
			state, err := availableAuthPackages(t.Context(), atmos, map[string]bool{"test/local-provider": true})
			require.Error(t, err, "registry-auth bootstrap must reject the same invalid local metadata")
			require.Nil(t, state)
			require.Zero(t, requests.Load())
			requireNoLock(t, atmos)
		})
	}
}

func TestOrdinaryCommandReportsCorruptLockWithoutRestoringPackages(t *testing.T) {
	atmos, requests := automaticFixture(t)
	invalid := []byte("spec: [invalid lock")
	require.NoError(t, os.WriteFile(pluginlock.Path(ProjectDirectory(atmos)), invalid, 0o600))
	cleanup, err := Prepare(t.Context(), atmos)
	require.Error(t, err)
	require.Nil(t, cleanup)
	require.Zero(t, requests.Load())
	after, err := os.ReadFile(pluginlock.Path(ProjectDirectory(atmos)))
	require.NoError(t, err)
	require.Equal(t, invalid, after, "bootstrap must not replace an unreadable lock")
}
