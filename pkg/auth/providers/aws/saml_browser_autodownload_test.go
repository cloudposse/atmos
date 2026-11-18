package aws

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// Legacy saml2aws CLI variable that Atmos used to set. Nothing reads it.
	legacyAutoDownloadEnv = "SAML2AWS_AUTO_BROWSER_DOWNLOAD"
	// Playwright host platform the browser download tests pin.
	testHostPlatform = "mac14-arm64"
)

// testBrowsersManifest mirrors the browsers.json of the bundled Playwright driver (1.64.1),
// including the WebKit revisionOverrides entry for macOS 14.
const testBrowsersManifest = `{
  "browsers": [
    {"name": "chromium", "revision": "1248"},
    {"name": "chromium-headless-shell", "revision": "1248"},
    {"name": "firefox", "revision": "1555"},
    {"name": "webkit", "revision": "2370", "revisionOverrides": {"mac14": "2251", "mac14-arm64": "2251"}}
  ]
}`

// useFakeDriverManifest points the Playwright driver lookup at a temp driver directory that holds
// testBrowsersManifest, isolates the browser cache, and pins the Playwright host platform.
// It returns the directory the driver installs browsers into.
func useFakeDriverManifest(t *testing.T, hostPlatform string) string {
	t.Helper()
	_, cacheDir := isolatedBrowserCacheDir(t)
	driverDir := t.TempDir()
	t.Setenv("PLAYWRIGHT_DRIVER_PATH", driverDir)
	t.Setenv("PLAYWRIGHT_CLI_PATH", "")
	t.Setenv("PLAYWRIGHT_HOST_PLATFORM_OVERRIDE", hostPlatform)
	require.NoError(t, os.MkdirAll(filepath.Join(driverDir, "package"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(driverDir, "package", playwrightBrowserManifest),
		[]byte(testBrowsersManifest), 0o600))
	return cacheDir
}

func newBrowserTypeProvider(t *testing.T, browserType string) *samlProvider {
	t.Helper()
	p, err := NewSAMLProvider("test", &schema.Provider{
		Kind:        "aws/saml",
		URL:         "https://idp.example.com/saml",
		Region:      "us-east-1",
		Driver:      "Browser",
		BrowserType: browserType,
	})
	require.NoError(t, err)
	return p.(*samlProvider)
}

// Regression: Playwright-managed browser types (chromium, firefox, webkit) are installed by
// Playwright, so a configured browser_type must not turn auto-download off. Only the browser that
// is launched counts: a Chromium install does not satisfy Firefox or WebKit.
func TestShouldDownloadBrowser_PlaywrightManagedBrowserTypes(t *testing.T) {
	tests := []struct {
		name         string
		browserType  string
		hostPlatform string
		installed    []string
		want         bool
	}{
		{name: "firefox missing while chromium is installed", browserType: "firefox", installed: []string{"chromium-1248"}, want: true},
		{name: "firefox installed", browserType: "firefox", installed: []string{"firefox-1555"}, want: false},
		{name: "firefox mixed case", browserType: "Firefox", installed: []string{"firefox-1555"}, want: false},
		{name: "firefox older revision only", browserType: "firefox", installed: []string{"firefox-1500"}, want: true},
		{name: "webkit missing", browserType: "webkit", hostPlatform: "ubuntu24.04-x64", installed: []string{"chromium-1248"}, want: true},
		{name: "webkit default revision installed", browserType: "webkit", hostPlatform: "ubuntu24.04-x64", installed: []string{"webkit-2370"}, want: false},
		{name: "explicit chromium missing", browserType: "chromium", installed: []string{"firefox-1555"}, want: true},
		{name: "explicit chromium installed", browserType: "chromium", installed: []string{"chromium-1248"}, want: false},
		{name: "default chromium installed", browserType: "", installed: []string{"chromium-1248"}, want: false},
		{name: "system chrome channel never downloads", browserType: "chrome", want: false},
		{name: "system msedge channel never downloads", browserType: "msedge-beta", want: false},
		{name: "unknown browser type never downloads", browserType: "netscape", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			platform := tt.hostPlatform
			if platform == "" {
				platform = "ubuntu24.04-x64"
			}
			cacheDir := useFakeDriverManifest(t, platform)
			for _, dir := range tt.installed {
				installFakeBrowser(t, cacheDir, dir)
			}

			assert.Equal(t, tt.want, newBrowserTypeProvider(t, tt.browserType).shouldDownloadBrowser())
		})
	}
}

// On a host platform with a revisionOverrides entry, Playwright installs WebKit into
// webkit_<platform>_special-<revision>, and the default webkit-<revision> does not satisfy it.
func TestShouldDownloadBrowser_WebKitRevisionOverride(t *testing.T) {
	t.Run("override revision installed", func(t *testing.T) {
		cacheDir := useFakeDriverManifest(t, testHostPlatform)
		installFakeBrowser(t, cacheDir, "webkit_mac14_arm64_special-2251")

		assert.False(t, newBrowserTypeProvider(t, "webkit").shouldDownloadBrowser())
	})

	t.Run("only the default revision installed", func(t *testing.T) {
		cacheDir := useFakeDriverManifest(t, testHostPlatform)
		installFakeBrowser(t, cacheDir, "webkit-2370")

		assert.True(t, newBrowserTypeProvider(t, "webkit").shouldDownloadBrowser())
	})
}

// A browser_executable_path is the user's own browser, so it never triggers a download.
func TestShouldDownloadBrowser_ExecutablePathNeverDownloads(t *testing.T) {
	useFakeDriverManifest(t, "ubuntu24.04-x64")
	p := newBrowserTypeProvider(t, "firefox")
	p.config.BrowserExecutablePath = filepath.Join(t.TempDir(), "firefox")

	assert.False(t, p.shouldDownloadBrowser())
}

// Regression: SAML2AWS_AUTO_BROWSER_DOWNLOAD is only read by the saml2aws CLI, which Atmos does not
// use. The setupBrowserAutomation function used to set and unset it process-wide, clobbering a value the user
// exported, although nothing read it.
func TestSetupBrowserAutomation_LeavesLegacyAutoDownloadEnvAlone(t *testing.T) {
	for _, download := range []bool{true, false} {
		t.Run(map[bool]string{true: "download enabled", false: "download disabled"}[download], func(t *testing.T) {
			isolatedBrowserCacheDir(t)
			t.Setenv(legacyAutoDownloadEnv, "user-value")

			p := newBrowserTypeProvider(t, "")
			p.config.DownloadBrowserDriver = download
			if !download {
				p.config.Driver = "GoogleApps"
			}
			require.NoError(t, p.setupBrowserAutomation())

			assert.Equal(t, "user-value", os.Getenv(legacyAutoDownloadEnv))
		})
	}

	t.Run("unset stays unset", func(t *testing.T) {
		isolatedBrowserCacheDir(t)
		t.Setenv(legacyAutoDownloadEnv, "")
		require.NoError(t, os.Unsetenv(legacyAutoDownloadEnv))

		p := newBrowserTypeProvider(t, "")
		p.config.DownloadBrowserDriver = true
		require.NoError(t, p.setupBrowserAutomation())

		_, set := os.LookupEnv(legacyAutoDownloadEnv)
		assert.False(t, set)
	})
}

// Regression: Environment() exported SAML2AWS_AUTO_BROWSER_DOWNLOAD to subprocesses although no
// consumer reads it.
func TestSAMLProvider_EnvironmentOmitsLegacyAutoDownloadEnv(t *testing.T) {
	for _, download := range []bool{true, false} {
		p := newBrowserTypeProvider(t, "")
		p.config.DownloadBrowserDriver = download

		env, err := p.Environment()
		require.NoError(t, err)
		assert.NotContains(t, env, legacyAutoDownloadEnv)
		assert.Equal(t, map[string]string{"AWS_DEFAULT_REGION": "us-east-1", "AWS_REGION": "us-east-1"}, env)
	}
}
