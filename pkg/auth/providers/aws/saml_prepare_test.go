package aws

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// With a seeded driver, preparing the Browser driver also patches the driver bundle.
func TestPrepareBrowserDriver_PatchesSeededDriver(t *testing.T) {
	isolatedBrowserCacheDir(t)
	bundlePath := seedFakePlaywrightDriver(t)

	p := &samlProvider{config: &schema.Provider{}}
	require.NoError(t, p.prepareBrowserDriver("Browser"))

	data, err := os.ReadFile(bundlePath)
	require.NoError(t, err)
	assert.Contains(t, string(data), `pageError.location?.url || ""`)
}

// Negative path: a patch failure is only logged, so authentication still proceeds with the unpatched driver.
func TestPrepareBrowserDriver_PatchFailureIsNotFatal(t *testing.T) {
	isolatedBrowserCacheDir(t)
	bundlePath := seedFakePlaywrightDriver(t)
	const unexpected = "console.log('a different playwright-core release');"
	require.NoError(t, os.WriteFile(bundlePath, []byte(unexpected), 0o644))

	p := &samlProvider{config: &schema.Provider{}}
	require.NoError(t, p.prepareBrowserDriver("Browser"))

	data, err := os.ReadFile(bundlePath)
	require.NoError(t, err)
	assert.Equal(t, unexpected, string(data), "the failed patch leaves the bundle untouched")
}

// An invalid browser executable stops the preparation before any driver is seeded.
func TestPrepareBrowserDriver_InvalidBrowserExecutable(t *testing.T) {
	isolatedBrowserCacheDir(t)
	driverDir := t.TempDir()
	t.Setenv("PLAYWRIGHT_DRIVER_PATH", driverDir)

	p := &samlProvider{config: &schema.Provider{BrowserExecutablePath: filepath.Join(t.TempDir(), "missing-browser")}}
	err := p.prepareBrowserDriver("Browser")
	require.ErrorIs(t, err, errUtils.ErrInvalidBrowserExecutable)
	assert.NoFileExists(t, filepath.Join(driverDir, "package", "cli.js"))
}

func TestHasValidPlaywrightDrivers_Layouts(t *testing.T) {
	p := &samlProvider{config: &schema.Provider{}}

	tests := []struct {
		name  string
		setup func(t *testing.T) string
		want  bool
	}{
		{
			name:  "missing directory",
			setup: func(t *testing.T) string { return filepath.Join(t.TempDir(), "missing") },
		},
		{
			name: "path is a file",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "file")
				require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
				return path
			},
		},
		{
			name:  "empty directory",
			setup: func(t *testing.T) string { return t.TempDir() },
		},
		{
			name: "only registry metadata and loose files",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(dir, ".links", "entry"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "stray-file"), []byte("x"), 0o600))
				return dir
			},
		},
		{
			name: "empty version directory",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(dir, "chromium-1"), 0o755))
				return dir
			},
		},
		{
			name: "version directory with a browser",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(dir, ".links"), 0o755))
				require.NoError(t, os.MkdirAll(filepath.Join(dir, "chromium-1"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "chromium-1", "chrome"), []byte("x"), 0o600))
				return dir
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, p.hasValidPlaywrightDrivers(tt.setup(t)))
		})
	}
}

// The directory the Playwright driver itself uses is checked first, ahead of the home-based defaults.
func TestPlaywrightBrowserPaths_RegistryDirFirst(t *testing.T) {
	home, registryDir := isolatedBrowserCacheDir(t)

	paths := (&samlProvider{config: &schema.Provider{}}).playwrightBrowserPaths()
	require.NotEmpty(t, paths)
	assert.Equal(t, registryDir, paths[0])
	assert.Contains(t, paths, filepath.Join(home, ".cache", playwrightCacheDir))
	assert.Contains(t, paths, filepath.Join(home, "Library", "Caches", playwrightCacheDir))
	assert.Contains(t, paths, filepath.Join(home, "AppData", "Local", playwrightCacheDir))
}

// Regression: the config and the download decision use the driver selected at the start of the attempt.
// Re-selecting could pick Browser after another process installed a browser mid-attempt, reaching the
// Browser client without its preparation and integrity check.
func TestCreateSAMLConfigAndLoginDetails_UseSelectedDriver(t *testing.T) {
	_, cacheDir := isolatedBrowserCacheDir(t)
	// A browser is installed now, so a fresh selection would pick Browser, but not the required revision.
	stubRequiredChromiumRevision(t, "1248")
	installFakeBrowser(t, cacheDir, "chromium-1000")

	p := &samlProvider{
		name:   "okta",
		url:    "https://example.okta.com/app/saml",
		region: "us-east-1",
		config: &schema.Provider{},
	}
	require.Equal(t, "Browser", p.getDriver(), "precondition: a fresh selection now picks Browser")

	assert.Equal(t, "Okta", p.createSAMLConfig("Okta").Provider)
	assert.False(t, p.createLoginDetails("Okta").DownloadBrowser, "a non-Browser attempt never downloads a browser")

	// Negative path: the Browser attempt still downloads the missing required revision.
	assert.Equal(t, "Browser", p.createSAMLConfig("Browser").Provider)
	assert.True(t, p.createLoginDetails("Browser").DownloadBrowser)
}
