package aws

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// unpatchedCoreBundle is the driver bundle code playwright-go rewrites in DownloadDriver.
const unpatchedCoreBundle = `location: {
  url: pageError.location.url,
  line: pageError.location.lineNumber,
  column: pageError.location.columnNumber
}`

// seedFakePlaywrightDriver creates a driver directory with cli.js and an unpatched bundle,
// and makes the test binary act as its Node.js runtime. It returns the bundle path.
func seedFakePlaywrightDriver(t *testing.T) string {
	t.Helper()
	driverDir := t.TempDir()
	t.Setenv("PLAYWRIGHT_DRIVER_PATH", driverDir)
	t.Setenv("PLAYWRIGHT_CLI_PATH", "")

	exe, err := os.Executable()
	require.NoError(t, err)
	t.Setenv("PLAYWRIGHT_NODEJS_PATH", exe)
	driver, err := playwright.NewDriver(&playwright.RunOptions{DriverDirectory: driverDir})
	require.NoError(t, err)
	t.Setenv(fakePlaywrightNodeEnv, driver.Version)

	require.NoError(t, os.MkdirAll(filepath.Join(driverDir, "package", "lib"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(driverDir, "package", "cli.js"), []byte("cli"), 0o644))
	bundlePath := filepath.Join(driverDir, "package", "lib", "coreBundle.js")
	require.NoError(t, os.WriteFile(bundlePath, []byte(unpatchedCoreBundle), 0o644))
	return bundlePath
}

// Prerequisite: the test binary answers the driver's --version check, so the patch test below
// exercises playwright-go's real up-to-date check instead of failing on a missing Node.js.
func TestFakePlaywrightNodeAnswersVersionCheck(t *testing.T) {
	seedFakePlaywrightDriver(t)

	driver, err := playwright.NewDriver(&playwright.RunOptions{})
	require.NoError(t, err)
	out, err := driver.Command("--version").Output()
	require.NoError(t, err)
	assert.Contains(t, string(out), driver.Version)
}

// Regression: playwright-go patches the driver bundle only in DownloadDriver (called by
// Install), never in Run. A pre-seeded driver whose browser download is skipped (browser
// already installed, or a custom browser_type) ran unpatched.
func TestPatchPlaywrightDriver_PatchesSeededBundle(t *testing.T) {
	bundlePath := seedFakePlaywrightDriver(t)

	require.NoError(t, patchPlaywrightDriver())

	data, err := os.ReadFile(bundlePath)
	require.NoError(t, err)
	assert.Contains(t, string(data), `pageError.location?.url || ""`)
	assert.Contains(t, string(data), "pageError.location?.lineNumber || 0")
	assert.Contains(t, string(data), "pageError.location?.columnNumber || 0")

	// Patching is idempotent.
	require.NoError(t, patchPlaywrightDriver())
}

// Without a driver, nothing is patched and nothing is downloaded.
func TestPatchPlaywrightDriver_NoDriverIsNoop(t *testing.T) {
	driverDir := t.TempDir()
	t.Setenv("PLAYWRIGHT_DRIVER_PATH", driverDir)
	t.Setenv("PLAYWRIGHT_CLI_PATH", "")
	// An unreachable registry proves no download is attempted.
	t.Setenv("PLAYWRIGHT_GO_NPM_REGISTRY", "http://127.0.0.1:1")

	require.NoError(t, patchPlaywrightDriver())
	assert.NoFileExists(t, filepath.Join(driverDir, "package", "cli.js"))
}

// An external PLAYWRIGHT_CLI_PATH driver is left alone, matching playwright-go.
func TestPatchPlaywrightDriver_ExternalCLIIsNoop(t *testing.T) {
	bundlePath := seedFakePlaywrightDriver(t)
	t.Setenv("PLAYWRIGHT_CLI_PATH", filepath.Join(t.TempDir(), "cli.js"))

	require.NoError(t, patchPlaywrightDriver())

	data, err := os.ReadFile(bundlePath)
	require.NoError(t, err)
	assert.Equal(t, unpatchedCoreBundle, string(data))
}

// A bundle without the expected code reports a seed error and is left untouched.
func TestPatchPlaywrightDriver_UnexpectedBundleFails(t *testing.T) {
	bundlePath := seedFakePlaywrightDriver(t)
	const unexpected = "console.log('a different playwright-core release');"
	require.NoError(t, os.WriteFile(bundlePath, []byte(unexpected), 0o644))

	err := patchPlaywrightDriver()
	require.ErrorIs(t, err, errUtils.ErrPlaywrightDriverSeed)
	require.ErrorIs(t, err, errUtils.ErrPlaywrightDriverBundle)

	data, readErr := os.ReadFile(bundlePath)
	require.NoError(t, readErr)
	assert.Equal(t, unexpected, string(data))
	assertNoStagedBundles(t, filepath.Dir(bundlePath))
}

// Regression: playwright-go rewrote the bundle in place, so a failed write truncated the live copy
// and concurrent readers could see a partial file. The patch is now published by renaming a staged
// file, so the patched bundle is a different file than the one that was patched.
func TestPatchPlaywrightDriver_PublishesAtomically(t *testing.T) {
	bundlePath := seedFakePlaywrightDriver(t)
	before := statFileIdentity(t, bundlePath)

	require.NoError(t, patchPlaywrightDriver())

	after := statFileIdentity(t, bundlePath)
	assert.False(t, os.SameFile(before, after), "the bundle must be replaced, not rewritten in place")
	assert.Equal(t, before.Mode().Perm(), after.Mode().Perm(), "the bundle keeps its file mode")
	assertNoStagedBundles(t, filepath.Dir(bundlePath))
}

// Negative path: an already patched bundle is not rewritten at all.
func TestPatchPlaywrightDriver_PatchedBundleIsNotRewritten(t *testing.T) {
	bundlePath := seedFakePlaywrightDriver(t)
	require.NoError(t, patchPlaywrightDriver())
	before := statFileIdentity(t, bundlePath)

	require.NoError(t, patchPlaywrightDriver())

	after := statFileIdentity(t, bundlePath)
	assert.True(t, os.SameFile(before, after))
}

// statFileIdentity stats path and captures the file's identity immediately. On Windows, os.Stat
// does not record the file ID; os.SameFile loads it from the path on its first call. Comparing a
// stat taken before a file was replaced would then read the replacement's ID for both sides.
func statFileIdentity(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.True(t, os.SameFile(info, info), "the file ID of %s must be readable", path)
	return info
}

// A missing bundle next to a seeded CLI is reported, and nothing is created.
func TestPatchPlaywrightDriver_MissingBundleFails(t *testing.T) {
	bundlePath := seedFakePlaywrightDriver(t)
	require.NoError(t, os.Remove(bundlePath))

	err := patchPlaywrightDriver()
	require.ErrorIs(t, err, errUtils.ErrPlaywrightDriverSeed)
	require.ErrorIs(t, err, errUtils.ErrPlaywrightDriverPatch)
	require.ErrorIs(t, err, fs.ErrNotExist)
	assert.NoFileExists(t, bundlePath)
}

// The bundle helpers classify their own I/O errors, so a direct caller does not depend on
// patchPlaywrightDriver to add the sentinel, and the underlying cause stays reachable.
func TestPatchDriverBundle_IOErrorsWrapSentinel(t *testing.T) {
	t.Run("missing bundle", func(t *testing.T) {
		err := patchDriverBundle(filepath.Join(t.TempDir(), "coreBundle.js"))
		require.ErrorIs(t, err, errUtils.ErrPlaywrightDriverPatch)
		require.ErrorIs(t, err, fs.ErrNotExist)
		assert.NotErrorIs(t, err, errUtils.ErrPlaywrightDriverSeed)
	})

	t.Run("closed staged file", func(t *testing.T) {
		staged, err := os.CreateTemp(t.TempDir(), ".coreBundle-*.tmp")
		require.NoError(t, err)
		require.NoError(t, staged.Close())

		err = writeStagedBundle(staged, []byte(unpatchedCoreBundle), 0o644)
		require.ErrorIs(t, err, errUtils.ErrPlaywrightDriverPatch)
		require.ErrorIs(t, err, os.ErrClosed)
	})

	t.Run("unexpected content is not an I/O error", func(t *testing.T) {
		bundlePath := filepath.Join(t.TempDir(), "coreBundle.js")
		require.NoError(t, os.WriteFile(bundlePath, []byte("console.log('other');"), 0o644))

		err := patchDriverBundle(bundlePath)
		require.ErrorIs(t, err, errUtils.ErrPlaywrightDriverBundle)
		assert.NotErrorIs(t, err, errUtils.ErrPlaywrightDriverPatch)
	})
}

// assertNoStagedBundles fails if a staged patch file was left behind.
func assertNoStagedBundles(t *testing.T, dir string) {
	t.Helper()
	staged, err := filepath.Glob(filepath.Join(dir, ".coreBundle-*.tmp"))
	require.NoError(t, err)
	assert.Empty(t, staged)
}
