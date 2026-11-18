package aws

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/schema"
)

// stubRequiredChromiumRevision makes requiredBrowserDir report a fixed Chromium revision for the test.
// Other browsers report an unknown revision.
func stubRequiredChromiumRevision(t *testing.T, revision string) {
	t.Helper()
	original := requiredBrowserDir
	t.Cleanup(func() { requiredBrowserDir = original })
	requiredBrowserDir = func(browserName string) (string, error) {
		if browserName != chromiumBrowserName {
			return "", errUtils.ErrPlaywrightBrowserManifest
		}
		return chromiumBrowserName + "-" + revision, nil
	}
}

// isolatedBrowserCacheDir points HOME (and XDG_CACHE_HOME, and LOCALAPPDATA on Windows) at a
// temp dir, clears PLAYWRIGHT_BROWSERS_PATH, and returns the home directory and the Playwright
// browser directory the driver uses on the current OS inside it.
func isolatedBrowserCacheDir(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PLAYWRIGHT_BROWSERS_PATH", "")
	t.Setenv("npm_config_playwright_browsers_path", "")
	t.Setenv("npm_package_config_playwright_browsers_path", "")
	t.Setenv("INIT_CWD", "")
	xdgCache := filepath.Join(home, ".xdg-cache")
	t.Setenv("XDG_CACHE_HOME", xdgCache)
	homedir.Reset()
	t.Cleanup(homedir.Reset)

	switch runtime.GOOS {
	case "darwin":
		return home, filepath.Join(home, "Library", "Caches", playwrightCacheDir)
	case "linux":
		// Playwright honors XDG_CACHE_HOME on Linux.
		return home, filepath.Join(xdgCache, playwrightCacheDir)
	default:
		t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
		return home, filepath.Join(home, "AppData", "Local", playwrightCacheDir)
	}
}

// installFakeBrowser creates a fully installed browser directory: a binary plus the
// INSTALLATION_COMPLETE marker Playwright writes after extraction.
func installFakeBrowser(t *testing.T, cacheDir, dirName string) {
	t.Helper()
	browserDir := filepath.Join(cacheDir, dirName)
	require.NoError(t, os.MkdirAll(browserDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(browserDir, "chrome"), []byte("fake"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(browserDir, playwrightInstallMarker), nil, 0o644))
}

func newTestSAMLProvider(t *testing.T) *samlProvider {
	t.Helper()
	provider, err := NewSAMLProvider("test", &schema.Provider{
		Kind:   "aws/saml",
		URL:    "https://idp.example.com/saml",
		Region: "us-east-1",
	})
	require.NoError(t, err)
	return provider.(*samlProvider)
}

func TestReadBrowserDir(t *testing.T) {
	const manifest = `{"browsers":[
		{"name":"chromium-headless-shell","revision":"1"},
		{"name":"chromium","revision":"1248"},
		{"name":"firefox","revision":"1555"},
		{"name":"webkit","revision":"2370","revisionOverrides":{"mac14":"2251","mac14-arm64":"2251"}}]}`

	tests := []struct {
		name     string
		manifest string
		browser  string
		platform string
		want     string
		wantErr  bool
	}{
		{name: "chromium entry", manifest: manifest, browser: chromiumBrowserName, want: "chromium-1248"},
		{name: "firefox entry", manifest: manifest, browser: firefoxBrowserName, want: "firefox-1555"},
		{name: "webkit without override", manifest: manifest, browser: webkitBrowserName, platform: "ubuntu24.04-x64", want: "webkit-2370"},
		{name: "webkit override", manifest: manifest, browser: webkitBrowserName, platform: "mac14", want: "webkit_mac14_special-2251"},
		{name: "webkit override with arch", manifest: manifest, browser: webkitBrowserName, platform: "mac14-arm64", want: "webkit_mac14_arm64_special-2251"},
		{name: "unknown platform uses the default revision", manifest: manifest, browser: webkitBrowserName, want: "webkit-2370"},
		{name: "dashes in the name become underscores", manifest: manifest, browser: "chromium-headless-shell", want: "chromium_headless_shell-1"},
		{name: "no chromium entry", manifest: `{"browsers":[{"name":"firefox","revision":"1555"}]}`, browser: chromiumBrowserName, wantErr: true},
		{name: "empty revision", manifest: `{"browsers":[{"name":"chromium","revision":""}]}`, browser: chromiumBrowserName, wantErr: true},
		{name: "invalid json", manifest: `{`, browser: chromiumBrowserName, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), playwrightBrowserManifest)
			require.NoError(t, os.WriteFile(path, []byte(tt.manifest), 0o600))

			got, err := readBrowserDir(path, tt.browser, tt.platform)
			if tt.wantErr {
				require.ErrorIs(t, err, errUtils.ErrPlaywrightBrowserManifest)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReadBrowserDirMissingFile(t *testing.T) {
	_, err := readBrowserDir(filepath.Join(t.TempDir(), "missing.json"), chromiumBrowserName, "")
	require.ErrorIs(t, err, errUtils.ErrPlaywrightBrowserManifest)
	require.ErrorIs(t, err, os.ErrNotExist)
}

// The real lookup reads the manifest from the bundled driver directory.
func TestRequiredBrowserDirFromDriverDirectory(t *testing.T) {
	driverDir := t.TempDir()
	t.Setenv("PLAYWRIGHT_DRIVER_PATH", driverDir)
	t.Setenv("PLAYWRIGHT_CLI_PATH", "")
	require.NoError(t, os.MkdirAll(filepath.Join(driverDir, "package"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(driverDir, "package", playwrightBrowserManifest),
		[]byte(`{"browsers":[{"name":"chromium","revision":"4321"}]}`), 0o600))

	got, err := requiredBrowserDir(chromiumBrowserName)
	require.NoError(t, err)
	assert.Equal(t, "chromium-4321", got)
}

func TestRequiredChromiumInstalled(t *testing.T) {
	t.Run("required revision installed", func(t *testing.T) {
		_, cacheDir := isolatedBrowserCacheDir(t)
		stubRequiredChromiumRevision(t, "1248")
		installFakeBrowser(t, cacheDir, "chromium-1248")

		assert.True(t, newTestSAMLProvider(t).requiredBrowserInstalled(chromiumBrowserName))
	})

	// Regression: after a Playwright upgrade the old Chromium revision stays on disk,
	// but the new driver cannot launch it, so it must not count as installed.
	t.Run("only an older revision installed", func(t *testing.T) {
		_, cacheDir := isolatedBrowserCacheDir(t)
		stubRequiredChromiumRevision(t, "1248")
		installFakeBrowser(t, cacheDir, "chromium-1200")

		p := newTestSAMLProvider(t)
		assert.True(t, p.playwrightDriversInstalled(), "some browser is installed")
		assert.False(t, p.requiredBrowserInstalled(chromiumBrowserName), "but not the required revision")
		assert.True(t, p.shouldDownloadBrowser(p.getDriver()), "so the new revision must be downloaded")
	})

	t.Run("empty revision directory", func(t *testing.T) {
		_, cacheDir := isolatedBrowserCacheDir(t)
		stubRequiredChromiumRevision(t, "1248")
		require.NoError(t, os.MkdirAll(filepath.Join(cacheDir, "chromium-1248"), 0o755))

		assert.False(t, newTestSAMLProvider(t).requiredBrowserInstalled(chromiumBrowserName))
	})

	t.Run("revision unknown", func(t *testing.T) {
		_, cacheDir := isolatedBrowserCacheDir(t)
		installFakeBrowser(t, cacheDir, "chromium-1248")
		original := requiredBrowserDir
		t.Cleanup(func() { requiredBrowserDir = original })
		requiredBrowserDir = func(string) (string, error) { return "", errors.New("no manifest") }

		p := newTestSAMLProvider(t)
		assert.False(t, p.requiredBrowserInstalled(chromiumBrowserName))
		assert.True(t, p.shouldDownloadBrowser(p.getDriver()), "download when the required revision cannot be determined")
	})

	t.Run("required revision installed disables auto-download", func(t *testing.T) {
		_, cacheDir := isolatedBrowserCacheDir(t)
		stubRequiredChromiumRevision(t, "1248")
		installFakeBrowser(t, cacheDir, "chromium-1248")

		assert.False(t, shouldDownloadBrowserFor(newTestSAMLProvider(t)))
	})
}

// Regression: an interrupted download leaves chromium-<revision> with files but without
// Playwright's INSTALLATION_COMPLETE marker. It must not count as installed, otherwise the
// download is skipped and the launch fails on the partial browser.
func TestRequiredChromiumInstalled_PartialDownload(t *testing.T) {
	_, cacheDir := isolatedBrowserCacheDir(t)
	stubRequiredChromiumRevision(t, "1248")
	browserDir := filepath.Join(cacheDir, "chromium-1248")
	require.NoError(t, os.MkdirAll(browserDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(browserDir, "chrome"), []byte("partial"), 0o755))

	p := newTestSAMLProvider(t)
	assert.False(t, p.requiredBrowserInstalled(chromiumBrowserName))
	assert.True(t, p.shouldDownloadBrowser(p.getDriver()), "a partial download must be completed")
}

// Regression: PLAYWRIGHT_BROWSERS_PATH moves the browser directory for the driver, so the
// required revision must be looked up there and nowhere else.
func TestRequiredChromiumInstalled_PlaywrightBrowsersPath(t *testing.T) {
	t.Run("installed in PLAYWRIGHT_BROWSERS_PATH", func(t *testing.T) {
		isolatedBrowserCacheDir(t)
		stubRequiredChromiumRevision(t, "1248")
		browsersPath := t.TempDir()
		t.Setenv("PLAYWRIGHT_BROWSERS_PATH", browsersPath)
		installFakeBrowser(t, browsersPath, "chromium-1248")

		p := newTestSAMLProvider(t)
		assert.True(t, p.requiredBrowserInstalled(chromiumBrowserName))
		assert.False(t, p.shouldDownloadBrowser(p.getDriver()))
	})

	t.Run("installed only in the default directory the driver will not use", func(t *testing.T) {
		_, cacheDir := isolatedBrowserCacheDir(t)
		stubRequiredChromiumRevision(t, "1248")
		installFakeBrowser(t, cacheDir, "chromium-1248")
		t.Setenv("PLAYWRIGHT_BROWSERS_PATH", t.TempDir())

		p := newTestSAMLProvider(t)
		assert.False(t, p.requiredBrowserInstalled(chromiumBrowserName))
		assert.True(t, p.shouldDownloadBrowser(p.getDriver()))
	})

	t.Run("zero means the driver package directory", func(t *testing.T) {
		isolatedBrowserCacheDir(t)
		stubRequiredChromiumRevision(t, "1248")
		driverDir := t.TempDir()
		t.Setenv("PLAYWRIGHT_DRIVER_PATH", driverDir)
		t.Setenv("PLAYWRIGHT_CLI_PATH", "")
		t.Setenv("PLAYWRIGHT_BROWSERS_PATH", "0")
		installFakeBrowser(t, filepath.Join(driverDir, "package", playwrightLocalBrowsers), "chromium-1248")

		assert.True(t, newTestSAMLProvider(t).requiredBrowserInstalled(chromiumBrowserName))
	})
}

func TestPlaywrightDefaultCacheDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	homedir.Reset()
	t.Cleanup(homedir.Reset)
	xdgCache := filepath.Join(home, "xdg")
	localAppData := filepath.Join(home, "local")

	tests := []struct {
		name string
		goos string
		env  map[string]string
		want string
	}{
		{name: "linux default", goos: "linux", env: map[string]string{"XDG_CACHE_HOME": ""}, want: filepath.Join(home, ".cache")},
		{name: "linux XDG_CACHE_HOME", goos: "linux", env: map[string]string{"XDG_CACHE_HOME": xdgCache}, want: xdgCache},
		{name: "darwin ignores XDG_CACHE_HOME", goos: "darwin", env: map[string]string{"XDG_CACHE_HOME": xdgCache}, want: filepath.Join(home, "Library", "Caches")},
		{name: "windows default", goos: "windows", env: map[string]string{"LOCALAPPDATA": ""}, want: filepath.Join(home, "AppData", "Local")},
		{name: "windows LOCALAPPDATA", goos: "windows", env: map[string]string{"LOCALAPPDATA": localAppData}, want: localAppData},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			got, err := playwrightDefaultCacheDir(tt.goos)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	_, err := playwrightDefaultCacheDir("plan9")
	require.ErrorIs(t, err, errUtils.ErrUnsupportedPlatform)
}

// A relative PLAYWRIGHT_BROWSERS_PATH is resolved against the working directory, like the driver does.
func TestPlaywrightRegistryDirRelativeBrowsersPath(t *testing.T) {
	t.Setenv("PLAYWRIGHT_BROWSERS_PATH", "browsers")
	t.Setenv("INIT_CWD", "")
	cwd, err := os.Getwd()
	require.NoError(t, err)

	got, err := playwrightRegistryDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(cwd, "browsers"), got)
}

// Regression: a relative PLAYWRIGHT_BROWSERS_PATH resolves against INIT_CWD before the working directory.
func TestPlaywrightRegistryDirRelativeBrowsersPathUsesInitCwd(t *testing.T) {
	initCwd := t.TempDir()
	t.Setenv("PLAYWRIGHT_BROWSERS_PATH", "browsers")
	t.Setenv("INIT_CWD", initCwd)

	got, err := playwrightRegistryDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(initCwd, "browsers"), got)
}

// Regression: Playwright also reads the npm config forms of PLAYWRIGHT_BROWSERS_PATH, in order.
func TestPlaywrightRegistryDirNpmEnvFallbacks(t *testing.T) {
	configDir := t.TempDir()
	packageDir := t.TempDir()
	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)
	t.Setenv("LOCALAPPDATA", cacheHome)
	t.Setenv("HOME", cacheHome)
	t.Setenv("USERPROFILE", cacheHome)
	t.Setenv("PLAYWRIGHT_BROWSERS_PATH", "")
	t.Setenv("npm_config_playwright_browsers_path", configDir)
	t.Setenv("npm_package_config_playwright_browsers_path", packageDir)

	// An empty but defined PLAYWRIGHT_BROWSERS_PATH wins over the npm variables and selects the default registry.
	defaultDir, err := playwrightRegistryDir()
	require.NoError(t, err)
	assert.NotEqual(t, configDir, defaultDir)
	assert.NotEqual(t, packageDir, defaultDir)
	assert.Equal(t, playwrightCacheDir, filepath.Base(defaultDir))

	// With PLAYWRIGHT_BROWSERS_PATH undefined, the npm variables apply in order.
	require.NoError(t, os.Unsetenv("PLAYWRIGHT_BROWSERS_PATH"))
	got, err := playwrightRegistryDir()
	require.NoError(t, err)
	assert.Equal(t, configDir, got)

	// An empty but defined npm_config_ variable stops the lookup as well.
	t.Setenv("npm_config_playwright_browsers_path", "")
	got, err = playwrightRegistryDir()
	require.NoError(t, err)
	assert.Equal(t, defaultDir, got)

	require.NoError(t, os.Unsetenv("npm_config_playwright_browsers_path"))
	got, err = playwrightRegistryDir()
	require.NoError(t, err)
	assert.Equal(t, packageDir, got)
}

// An external PLAYWRIGHT_CLI_PATH moves browsers.json with cli.js.
func TestPlaywrightManifestPath(t *testing.T) {
	driverDir := t.TempDir()

	t.Setenv("PLAYWRIGHT_CLI_PATH", "")
	assert.Equal(t, filepath.Join(driverDir, "package", playwrightBrowserManifest), playwrightManifestPath(driverDir))

	cliDir := t.TempDir()
	t.Setenv("PLAYWRIGHT_CLI_PATH", filepath.Join(cliDir, "cli.js"))
	assert.Equal(t, filepath.Join(cliDir, playwrightBrowserManifest), playwrightManifestPath(driverDir))
}

// Regression: a relative default cache directory (XDG_CACHE_HOME=cache on Linux, LOCALAPPDATA on Windows)
// is resolved against INIT_CWD like the driver does, not against the working directory.
func TestPlaywrightRegistryDirRelativeDefaultCacheUsesInitCwd(t *testing.T) {
	switch runtime.GOOS {
	case "linux":
		t.Setenv("XDG_CACHE_HOME", "cache")
	case windowsOS:
		t.Setenv("LOCALAPPDATA", "cache")
	default:
		t.Skipf("the default cache directory is always absolute on %s", runtime.GOOS)
	}
	initCwd := t.TempDir()
	t.Setenv("PLAYWRIGHT_BROWSERS_PATH", "")
	t.Setenv("INIT_CWD", initCwd)

	got, err := playwrightRegistryDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(initCwd, "cache", playwrightCacheDir), got)

	// Negative path: without INIT_CWD the working directory is used.
	t.Setenv("INIT_CWD", "")
	cwd, err := os.Getwd()
	require.NoError(t, err)
	got, err = playwrightRegistryDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(cwd, "cache", playwrightCacheDir), got)
}

// Regression: Playwright also reads the npm config forms of INIT_CWD, in order.
func TestPlaywrightRegistryDirInitCwdNpmEnvFallbacks(t *testing.T) {
	configDir := t.TempDir()
	packageDir := t.TempDir()
	t.Setenv("PLAYWRIGHT_BROWSERS_PATH", "browsers")
	require.NoError(t, os.Unsetenv("INIT_CWD"))
	t.Setenv("npm_config_init_cwd", configDir)
	t.Setenv("npm_package_config_init_cwd", packageDir)

	got, err := playwrightRegistryDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(configDir, "browsers"), got)

	require.NoError(t, os.Unsetenv("npm_config_init_cwd"))
	got, err = playwrightRegistryDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(packageDir, "browsers"), got)

	// An empty INIT_CWD falls back to the working directory, as in Playwright.
	t.Setenv("INIT_CWD", "")
	cwd, err := os.Getwd()
	require.NoError(t, err)
	got, err = playwrightRegistryDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(cwd, "browsers"), got)
}

// Regression: Node runs the target of a symlinked cli.js, so the package manifest and the
// PLAYWRIGHT_BROWSERS_PATH=0 directory belong to the link target, not the link's directory.
func TestPlaywrightManifestPathResolvesSymlinkedCLI(t *testing.T) {
	targetDir := t.TempDir()
	targetCLI := filepath.Join(targetDir, "cli.js")
	require.NoError(t, os.WriteFile(targetCLI, []byte("cli"), 0o644))
	linkCLI := filepath.Join(t.TempDir(), "cli.js")
	if err := os.Symlink(targetCLI, linkCLI); err != nil {
		t.Skipf("cannot create a symlink on this host: %v", err)
	}
	realDir, err := filepath.EvalSymlinks(targetDir)
	require.NoError(t, err)

	t.Setenv("PLAYWRIGHT_CLI_PATH", linkCLI)
	assert.Equal(t, filepath.Join(realDir, playwrightBrowserManifest), playwrightManifestPath(t.TempDir()))

	t.Setenv("PLAYWRIGHT_BROWSERS_PATH", "0")
	t.Setenv("PLAYWRIGHT_DRIVER_PATH", t.TempDir())
	got, err := playwrightRegistryDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(realDir, playwrightLocalBrowsers), got)
}
