package aws

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
				versionDir := filepath.Join(dir, "1.47.2")
				require.NoError(t, os.Mkdir(versionDir, 0755))
				return dir
			},
			expected: false,
			wantLog:  "no browser binaries",
		},
		{
			name: "directory with version containing files returns true",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				versionDir := filepath.Join(dir, "1.47.2")
				require.NoError(t, os.Mkdir(versionDir, 0755))

				// Create a fake browser binary.
				browserFile := filepath.Join(versionDir, "chromium-1234")
				require.NoError(t, os.Mkdir(browserFile, 0755))

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
				emptyVersion := filepath.Join(dir, "1.46.0")
				require.NoError(t, os.Mkdir(emptyVersion, 0755))

				// Valid version.
				validVersion := filepath.Join(dir, "1.47.2")
				require.NoError(t, os.Mkdir(validVersion, 0755))
				browserFile := filepath.Join(validVersion, "chromium-1234")
				require.NoError(t, os.Mkdir(browserFile, 0755))

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
				playwrightDir := filepath.Join(homeDir, ".cache", "ms-playwright", "1.47.2")
				require.NoError(t, os.MkdirAll(playwrightDir, 0755))

				// Create fake browser.
				browserFile := filepath.Join(playwrightDir, "chromium-1234")
				require.NoError(t, os.Mkdir(browserFile, 0755))

				return homeDir
			},
			expected: true,
		},
		{
			name:                  "valid drivers in macOS location returns true",
			downloadBrowserDriver: false,
			setup: func(t *testing.T) string {
				homeDir := t.TempDir()
				playwrightDir := filepath.Join(homeDir, "Library", "Caches", "ms-playwright-go", "1.47.2")
				require.NoError(t, os.MkdirAll(playwrightDir, 0755))

				// Create fake browser.
				browserFile := filepath.Join(playwrightDir, "chromium-1234")
				require.NoError(t, os.Mkdir(browserFile, 0755))

				return homeDir
			},
			expected: true,
		},
		{
			name:                  "valid drivers in Windows location returns true",
			downloadBrowserDriver: false,
			setup: func(t *testing.T) string {
				homeDir := t.TempDir()
				playwrightDir := filepath.Join(homeDir, "AppData", "Local", "ms-playwright", "1.47.2")
				require.NoError(t, os.MkdirAll(playwrightDir, 0755))

				// Create fake browser.
				browserFile := filepath.Join(playwrightDir, "chromium-1234")
				require.NoError(t, os.Mkdir(browserFile, 0755))

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
				playwrightDir := filepath.Join(homeDir, "Library", "Caches", "ms-playwright-go", "1.47.2")
				require.NoError(t, os.MkdirAll(playwrightDir, 0755))

				return homeDir
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homeDir := tt.setup(t)

			// Temporarily override home directory.
			originalHome := os.Getenv("HOME")
			t.Setenv("HOME", homeDir)
			defer func() {
				if originalHome != "" {
					os.Setenv("HOME", originalHome)
				}
			}()

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
				playwrightDir := filepath.Join(homeDir, "Library", "Caches", "ms-playwright-go", "1.47.2")
				require.NoError(t, os.MkdirAll(playwrightDir, 0755))
				browserFile := filepath.Join(playwrightDir, "chromium-1234")
				require.NoError(t, os.Mkdir(browserFile, 0755))
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
				playwrightDir := filepath.Join(homeDir, "Library", "Caches", "ms-playwright-go", "1.47.2")
				require.NoError(t, os.MkdirAll(playwrightDir, 0755))
				browserFile := filepath.Join(playwrightDir, "chromium-1234")
				require.NoError(t, os.Mkdir(browserFile, 0755))
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
				playwrightDir := filepath.Join(homeDir, "Library", "Caches", "ms-playwright-go", "1.47.2")
				require.NoError(t, os.MkdirAll(playwrightDir, 0755))
				return homeDir
			},
			expected: "GoogleApps",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homeDir := tt.setup(t)

			// Override home directory.
			t.Setenv("HOME", homeDir)

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
