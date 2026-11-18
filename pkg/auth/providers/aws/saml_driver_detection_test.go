package aws

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestHasValidPlaywrightDrivers(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) string // Returns test directory path.
		expected bool
		wantLog  string // Expected log message substring.
	}{
		{
			name: "empty directory returns false",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				return dir
			},
			expected: false,
			wantLog:  "no browser binaries",
		},
		{
			name: "directory with empty version subdirectory returns false",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				versionDir := filepath.Join(dir, "chromium-1234")
				require.NoError(t, os.Mkdir(versionDir, 0o755))
				return dir
			},
			expected: false,
			wantLog:  "no browser binaries",
		},
		{
			name: "directory with version containing files returns true",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				// Create a realistic browser revision directory with a binary.
				browserDir := filepath.Join(dir, "chromium-1234")
				require.NoError(t, os.MkdirAll(browserDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(browserDir, "chrome"), []byte("fake"), 0o755))

				return dir
			},
			expected: true,
			wantLog:  "Found browser binaries",
		},
		{
			name: "directory with multiple versions returns true if any valid",
			setup: func(t *testing.T) string {
				dir := t.TempDir()

				// Empty version.
				emptyVersion := filepath.Join(dir, "chromium-1100")
				require.NoError(t, os.Mkdir(emptyVersion, 0o755))

				// Valid version.
				browserDir := filepath.Join(dir, "chromium-1234")
				require.NoError(t, os.MkdirAll(browserDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(browserDir, "chrome"), []byte("fake"), 0o755))

				return dir
			},
			expected: true,
			wantLog:  "Found browser binaries",
		},
		{
			name: "registry metadata only returns false",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				linksDir := filepath.Join(dir, ".links")
				require.NoError(t, os.Mkdir(linksDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(linksDir, "client"), []byte("meta"), 0o644))
				return dir
			},
			expected: false,
			wantLog:  "no browser binaries",
		},
		{
			// Regression: a registry holding only a Playwright tool is not an installed browser.
			name: "tool directories only returns false",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				for _, tool := range []string{"ffmpeg-1013", "winldd-1007"} {
					toolDir := filepath.Join(dir, tool)
					require.NoError(t, os.Mkdir(toolDir, 0o755))
					require.NoError(t, os.WriteFile(filepath.Join(toolDir, "tool"), []byte("fake"), 0o755))
				}
				return dir
			},
			expected: false,
			wantLog:  "no browser binaries",
		},
		{
			// An older browser revision stays eligible so the exact-revision check can trigger the upgrade.
			name: "tool next to an older browser revision returns true",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				toolDir := filepath.Join(dir, "ffmpeg-1013")
				require.NoError(t, os.Mkdir(toolDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(toolDir, "tool"), []byte("fake"), 0o755))
				browserDir := filepath.Join(dir, "chromium_headless_shell-1000")
				require.NoError(t, os.Mkdir(browserDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(browserDir, "headless_shell"), []byte("fake"), 0o755))
				return dir
			},
			expected: true,
			wantLog:  "Found browser binaries",
		},
		{
			name: "non-existent directory returns false",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "does-not-exist")
			},
			expected: false,
			wantLog:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDir := tt.setup(t)

			provider := &samlProvider{
				config: &schema.Provider{},
			}

			result := provider.hasValidPlaywrightDrivers(testDir)
			assert.Equal(t, tt.expected, result, "hasValidPlaywrightDrivers result mismatch")
		})
	}
}

func TestHasPlaywrightDriversOrCanDownload(t *testing.T) {
	tests := []struct {
		name                  string
		downloadBrowserDriver bool
		setup                 func(t *testing.T) string // Returns home directory.
		expected              bool
	}{
		{
			name:                  "explicit download enabled returns true",
			downloadBrowserDriver: true,
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			expected: true,
		},
		{
			name:                  "no drivers and no explicit download returns false",
			downloadBrowserDriver: false,
			setup: func(t *testing.T) string {
				// Return temp dir with no Playwright cache.
				return t.TempDir()
			},
			expected: false,
		},
		{
			name:                  "valid drivers in Linux location returns true",
			downloadBrowserDriver: false,
			setup: func(t *testing.T) string {
				homeDir := t.TempDir()
				playwrightDir := filepath.Join(homeDir, ".cache", "ms-playwright")
				require.NoError(t, os.MkdirAll(playwrightDir, 0o755))

				// Create fake browser.
				browserDir := filepath.Join(playwrightDir, "chromium-1234")
				require.NoError(t, os.MkdirAll(browserDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(browserDir, "chrome"), []byte("fake"), 0o755))

				return homeDir
			},
			expected: true,
		},
		{
			name:                  "valid drivers in macOS location returns true",
			downloadBrowserDriver: false,
			setup: func(t *testing.T) string {
				homeDir := t.TempDir()
				playwrightDir := filepath.Join(homeDir, "Library", "Caches", "ms-playwright")
				require.NoError(t, os.MkdirAll(playwrightDir, 0o755))

				// Create fake browser.
				browserDir := filepath.Join(playwrightDir, "chromium-1234")
				require.NoError(t, os.MkdirAll(browserDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(browserDir, "chrome"), []byte("fake"), 0o755))

				return homeDir
			},
			expected: true,
		},
		{
			name:                  "valid drivers in Windows location returns true",
			downloadBrowserDriver: false,
			setup: func(t *testing.T) string {
				homeDir := t.TempDir()
				playwrightDir := filepath.Join(homeDir, "AppData", "Local", "ms-playwright")
				require.NoError(t, os.MkdirAll(playwrightDir, 0o755))

				// Create fake browser.
				browserDir := filepath.Join(playwrightDir, "chromium-1234")
				require.NoError(t, os.MkdirAll(browserDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(browserDir, "chrome"), []byte("fake"), 0o755))

				return homeDir
			},
			expected: true,
		},
		{
			name:                  "empty Playwright directory returns false",
			downloadBrowserDriver: false,
			setup: func(t *testing.T) string {
				homeDir := t.TempDir()
				// Create empty version directory (like the bug we fixed).
				playwrightDir := filepath.Join(homeDir, "Library", "Caches", "ms-playwright")
				require.NoError(t, os.MkdirAll(playwrightDir, 0o755))

				return homeDir
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homeDir := tt.setup(t)

			isolateDriverDetection(t, homeDir)

			provider := &samlProvider{
				config: &schema.Provider{
					DownloadBrowserDriver: tt.downloadBrowserDriver,
				},
			}

			result := provider.hasPlaywrightDriversOrCanDownload()
			assert.Equal(t, tt.expected, result, "hasPlaywrightDriversOrCanDownload result mismatch")
		})
	}
}

func TestGetDriver_WithPlaywrightDrivers(t *testing.T) {
	tests := []struct {
		name           string
		explicitDriver string
		url            string
		setup          func(t *testing.T) string // Returns home directory.
		expected       string
	}{
		{
			name:           "explicit driver overrides detection",
			explicitDriver: "GoogleApps",
			url:            "https://accounts.google.com/saml",
			setup: func(t *testing.T) string {
				// Even with valid drivers, explicit config wins.
				homeDir := t.TempDir()
				playwrightDir := filepath.Join(homeDir, "Library", "Caches", "ms-playwright")
				require.NoError(t, os.MkdirAll(playwrightDir, 0o755))
				browserDir := filepath.Join(playwrightDir, "chromium-1234")
				require.NoError(t, os.MkdirAll(browserDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(browserDir, "chrome"), []byte("fake"), 0o755))
				return homeDir
			},
			expected: "GoogleApps",
		},
		{
			name:           "with valid drivers uses Browser",
			explicitDriver: "",
			url:            "https://accounts.google.com/saml",
			setup: func(t *testing.T) string {
				homeDir := t.TempDir()
				playwrightDir := filepath.Join(homeDir, "Library", "Caches", "ms-playwright")
				require.NoError(t, os.MkdirAll(playwrightDir, 0o755))
				browserDir := filepath.Join(playwrightDir, "chromium-1234")
				require.NoError(t, os.MkdirAll(browserDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(browserDir, "chrome"), []byte("fake"), 0o755))
				return homeDir
			},
			expected: "Browser",
		},
		{
			name:           "without drivers falls back to GoogleApps",
			explicitDriver: "",
			url:            "https://accounts.google.com/saml",
			setup: func(t *testing.T) string {
				return t.TempDir() // No drivers.
			},
			expected: "GoogleApps",
		},
		{
			name:           "without drivers falls back to Okta",
			explicitDriver: "",
			url:            "https://mycompany.okta.com/saml",
			setup: func(t *testing.T) string {
				return t.TempDir() // No drivers.
			},
			expected: "Okta",
		},
		{
			name:           "empty Playwright dir falls back to GoogleApps",
			explicitDriver: "",
			url:            "https://accounts.google.com/saml",
			setup: func(t *testing.T) string {
				homeDir := t.TempDir()
				// Create empty version directory (the bug scenario).
				playwrightDir := filepath.Join(homeDir, "Library", "Caches", "ms-playwright")
				require.NoError(t, os.MkdirAll(playwrightDir, 0o755))
				return homeDir
			},
			expected: "GoogleApps",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homeDir := tt.setup(t)

			isolateDriverDetection(t, homeDir)

			provider := &samlProvider{
				url: tt.url,
				config: &schema.Provider{
					Driver: tt.explicitDriver,
				},
			}

			result := provider.getDriver()
			assert.Equal(t, tt.expected, result, "getDriver result mismatch")
		})
	}
}

// isolateDriverDetection points every input of Playwright browser detection at test-owned directories.
// HOME/USERPROFILE is homeDir, so fixtures created under it stay reachable through the home-based
// locations. The Playwright registry (PLAYWRIGHT_BROWSERS_PATH), the Atmos and XDG cache directories and,
// on Windows, LOCALAPPDATA point at empty directories, so browsers installed on the host do not leak in.
func isolateDriverDetection(t *testing.T, homeDir string) {
	t.Helper()
	t.Setenv("HOME", homeDir)        // Linux/macOS.
	t.Setenv("USERPROFILE", homeDir) // Windows.
	t.Setenv("PLAYWRIGHT_BROWSERS_PATH", t.TempDir())
	t.Setenv("npm_config_playwright_browsers_path", "")
	t.Setenv("npm_package_config_playwright_browsers_path", "")
	t.Setenv("INIT_CWD", "")
	emptyCache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", emptyCache)
	t.Setenv("ATMOS_XDG_CACHE_HOME", emptyCache)
	t.Setenv("LOCALAPPDATA", t.TempDir())

	// Clear homedir cache to ensure environment variables take effect.
	t.Cleanup(homedir.Reset)
	homedir.Reset()
}

// Prerequisite: a browser in the Playwright registry is detected, so the isolation above is what keeps a
// browser installed on the host out of the negative cases.
func TestIsolateDriverDetection_RegistryBrowserIsDetected(t *testing.T) {
	isolateDriverDetection(t, t.TempDir())
	registryDir := t.TempDir()
	t.Setenv("PLAYWRIGHT_BROWSERS_PATH", registryDir)
	browserDir := filepath.Join(registryDir, "chromium-1234")
	require.NoError(t, os.MkdirAll(browserDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(browserDir, "chrome"), []byte("fake"), 0o755))

	provider := &samlProvider{config: &schema.Provider{}}
	assert.True(t, provider.hasPlaywrightDriversOrCanDownload())
}
