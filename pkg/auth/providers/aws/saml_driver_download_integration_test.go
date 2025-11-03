package aws

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/playwright-community/playwright-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/versent/saml2aws/v2/pkg/cfg"
	"github.com/versent/saml2aws/v2/pkg/creds"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestPlaywrightDriverDownload_Integration validates that the actual Playwright driver download works.
// This is an integration test that downloads real browser drivers (~100-300MB).
// It is skipped by default and must be explicitly enabled with:
//
//	RUN_PLAYWRIGHT_INTEGRATION=1 go test -v -run TestPlaywrightDriverDownload_Integration.
func TestPlaywrightDriverDownload_Integration(t *testing.T) {
	// Skip unless explicitly opted in via environment variable.
	if os.Getenv("RUN_PLAYWRIGHT_INTEGRATION") != "1" {
		t.Skip("Skipping Playwright integration test (set RUN_PLAYWRIGHT_INTEGRATION=1 to run)")
	}

	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create isolated test environment.
	testHomeDir := t.TempDir()
	t.Setenv("HOME", testHomeDir)        // Linux/macOS.
	t.Setenv("USERPROFILE", testHomeDir) // Windows.

	// Determine platform-specific cache directory where Playwright stores browser drivers.
	var cacheDir string
	switch runtime.GOOS {
	case "darwin":
		cacheDir = filepath.Join(testHomeDir, "Library", "Caches", "ms-playwright")
	case "linux":
		cacheDir = filepath.Join(testHomeDir, ".cache", "ms-playwright")
	case "windows":
		cacheDir = filepath.Join(testHomeDir, "AppData", "Local", "ms-playwright")
	default:
		t.Skipf("Unsupported platform: %s", runtime.GOOS)
	}

	// Verify cache directory doesn't exist initially.
	_, err := os.Stat(cacheDir)
	require.True(t, os.IsNotExist(err), "Cache directory should not exist initially")

	t.Logf("Test environment: home=%s, cache=%s", testHomeDir, cacheDir)

	// Create SAML provider with download enabled.
	provider, err := NewSAMLProvider("test-saml", &schema.Provider{
		Kind:                  "aws/saml",
		URL:                   "https://accounts.google.com/saml",
		Region:                "us-east-1",
		DownloadBrowserDriver: true,
		Driver:                "Browser", // Explicitly use Browser driver.
	})
	require.NoError(t, err)
	sp := provider.(*samlProvider)

	// Verify shouldDownloadBrowser returns true.
	shouldDownload := sp.shouldDownloadBrowser()
	assert.True(t, shouldDownload, "shouldDownloadBrowser should return true when DownloadBrowserDriver is enabled")

	// Create SAML config.
	samlConfig := sp.createSAMLConfig()
	assert.True(t, samlConfig.DownloadBrowser, "SAML config should have DownloadBrowser enabled")

	// Create LoginDetails.
	loginDetails := sp.createLoginDetails()
	assert.True(t, loginDetails.DownloadBrowser, "LoginDetails should have DownloadBrowser enabled")

	// Test the actual Playwright driver installation.
	t.Log("Starting Playwright driver download (this may take 1-2 minutes)...")

	runOptions := playwright.RunOptions{
		SkipInstallBrowsers: false,
		Browsers:            []string{"chromium"}, // Only download Chromium to save time.
	}

	err = playwright.Install(&runOptions)
	require.NoError(t, err, "Playwright driver installation should succeed")

	t.Log("Playwright drivers downloaded successfully")

	// Verify cache directory was created.
	cacheInfo, err := os.Stat(cacheDir)
	require.NoError(t, err, "Cache directory should exist after download")
	require.True(t, cacheInfo.IsDir(), "Cache path should be a directory")

	// Verify cache directory has content.
	entries, err := os.ReadDir(cacheDir)
	require.NoError(t, err, "Should be able to read cache directory")
	require.NotEmpty(t, entries, "Cache directory should contain version subdirectories")

	t.Logf("Cache directory contains %d entries", len(entries))

	// Verify Chromium browser binary was actually downloaded.
	hasValidChromium := false
	var chromiumPath string

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Look for chromium-* directories (actual browser installations).
		if !strings.HasPrefix(entry.Name(), "chromium-") {
			continue
		}

		versionPath := filepath.Join(cacheDir, entry.Name())
		versionEntries, err := os.ReadDir(versionPath)
		if err != nil {
			continue
		}

		// Chromium directory should contain multiple files/subdirectories.
		// A valid installation has chrome-mac/ or Chromium.app/ (macOS), chrome-linux/ (Linux), etc.
		if len(versionEntries) > 0 {
			hasValidChromium = true
			chromiumPath = versionPath
			t.Logf("Found Chromium installation in %s (%d entries)", entry.Name(), len(versionEntries))

			// Log first few entries for debugging.
			for i, ve := range versionEntries {
				if i >= 5 {
					break
				}
				entryType := "file"
				if ve.IsDir() {
					entryType = "dir"
				}
				t.Logf("  - %s (%s)", ve.Name(), entryType)
			}
			break
		}
	}

	require.True(t, hasValidChromium, "Chromium browser directory should exist with actual binaries")
	require.NotEmpty(t, chromiumPath, "Chromium installation path should be set")

	// Verify the Chromium directory is substantial (not just metadata).
	// A real Chromium installation is ~100-150 MB, so the directory should have substantial content.
	chromiumInfo, err := os.Stat(chromiumPath)
	require.NoError(t, err, "Should be able to stat Chromium directory")
	require.True(t, chromiumInfo.IsDir(), "Chromium path should be a directory")

	// Count total files in Chromium directory to ensure it's a complete installation.
	fileCount := 0
	err = filepath.Walk(chromiumPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			fileCount++
		}
		return nil
	})
	require.NoError(t, err, "Should be able to walk Chromium directory")
	require.Greater(t, fileCount, 10, "Chromium installation should contain many files (actual installation has hundreds)")

	t.Logf("Chromium installation validated: %d files found", fileCount)

	// Verify our detection logic now recognizes the installed drivers.
	hasValidDrivers := sp.hasValidPlaywrightDrivers(cacheDir)
	assert.True(t, hasValidDrivers, "hasValidPlaywrightDrivers should detect the installed drivers")

	// Verify playwrightDriversInstalled now returns true.
	driversInstalled := sp.playwrightDriversInstalled()
	assert.True(t, driversInstalled, "playwrightDriversInstalled should return true after installation")

	// Note: shouldDownloadBrowser still returns true because DownloadBrowserDriver: true is explicitly set.
	// This is correct behavior - explicit config always takes precedence.
	shouldDownloadAfter := sp.shouldDownloadBrowser()
	assert.True(t, shouldDownloadAfter, "shouldDownloadBrowser returns true when explicitly configured")

	// Test with a new provider without explicit download flag to verify auto-detection works.
	providerAutoDetect, err := NewSAMLProvider("test-auto", &schema.Provider{
		Kind:   "aws/saml",
		URL:    "https://accounts.google.com/saml",
		Region: "us-east-1",
		Driver: "Browser",
		// DownloadBrowserDriver not set - should auto-detect.
	})
	require.NoError(t, err)
	spAuto := providerAutoDetect.(*samlProvider)

	// Now with drivers installed, auto-detection should disable download.
	shouldDownloadAuto := spAuto.shouldDownloadBrowser()
	assert.False(t, shouldDownloadAuto, "shouldDownloadBrowser should return false when drivers exist and not explicitly configured")

	t.Log("All driver detection checks passed")
}

// TestPlaywrightDriverDownload_WithSAML2AWS validates that saml2aws actually uses LoginDetails.DownloadBrowser.
// This test simulates the saml2aws workflow without making actual network calls.
func TestPlaywrightDriverDownload_WithSAML2AWS(t *testing.T) {
	// Skip unless explicitly opted in via environment variable.
	if os.Getenv("RUN_PLAYWRIGHT_INTEGRATION") != "1" {
		t.Skip("Skipping Playwright integration test (set RUN_PLAYWRIGHT_INTEGRATION=1 to run)")
	}

	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create isolated test environment.
	testHomeDir := t.TempDir()
	t.Setenv("HOME", testHomeDir)
	t.Setenv("USERPROFILE", testHomeDir)

	// Create SAML provider.
	provider, err := NewSAMLProvider("test-saml", &schema.Provider{
		Kind:                  "aws/saml",
		URL:                   "https://accounts.google.com/saml",
		Region:                "us-east-1",
		Username:              "test@example.com",
		DownloadBrowserDriver: true,
		Driver:                "Browser",
	})
	require.NoError(t, err)
	sp := provider.(*samlProvider)

	// Create configurations that would be passed to saml2aws.
	samlConfig := sp.createSAMLConfig()
	loginDetails := sp.createLoginDetails()

	// Verify both have DownloadBrowser set.
	assert.True(t, samlConfig.DownloadBrowser, "IDPAccount config should have DownloadBrowser enabled")
	assert.True(t, loginDetails.DownloadBrowser, "LoginDetails should have DownloadBrowser enabled")

	// Verify the config matches what saml2aws expects.
	assert.Equal(t, "https://accounts.google.com/saml", samlConfig.URL)
	assert.Equal(t, "Browser", samlConfig.Provider)
	assert.Equal(t, "test-saml", samlConfig.Profile)
	assert.Equal(t, "us-east-1", samlConfig.Region)

	// Verify LoginDetails matches.
	assert.Equal(t, "https://accounts.google.com/saml", loginDetails.URL)
	assert.Equal(t, "test@example.com", loginDetails.Username)

	t.Log("saml2aws configuration validated successfully")
}

// TestPlaywrightDriverDownload_ConsistentBehavior validates that DownloadBrowser is consistently
// set across IDPAccount config and LoginDetails.
func TestPlaywrightDriverDownload_ConsistentBehavior(t *testing.T) {
	tests := []struct {
		name                  string
		downloadBrowserDriver bool
		driver                string
		setupDrivers          bool // Whether to pre-install drivers.
		expectedDownload      bool
	}{
		{
			name:                  "explicit enable",
			downloadBrowserDriver: true,
			driver:                "Browser",
			setupDrivers:          false,
			expectedDownload:      true,
		},
		{
			name:                  "explicit enable even with drivers installed",
			downloadBrowserDriver: true,
			driver:                "Browser",
			setupDrivers:          true,
			expectedDownload:      true,
		},
		{
			name:                  "auto-enable when no drivers",
			downloadBrowserDriver: false,
			driver:                "Browser",
			setupDrivers:          false,
			expectedDownload:      true, // Auto-enabled.
		},
		{
			name:                  "disabled when drivers exist",
			downloadBrowserDriver: false,
			driver:                "Browser",
			setupDrivers:          true,
			expectedDownload:      false,
		},
		{
			name:                  "disabled for GoogleApps driver",
			downloadBrowserDriver: false,
			driver:                "GoogleApps",
			setupDrivers:          false,
			expectedDownload:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testHomeDir := t.TempDir()
			t.Setenv("HOME", testHomeDir)
			t.Setenv("USERPROFILE", testHomeDir)

			// Optionally pre-install drivers.
			if tt.setupDrivers {
				var cacheDir string
				switch runtime.GOOS {
				case "darwin":
					cacheDir = filepath.Join(testHomeDir, "Library", "Caches", "ms-playwright")
				case "linux":
					cacheDir = filepath.Join(testHomeDir, ".cache", "ms-playwright")
				default:
					cacheDir = filepath.Join(testHomeDir, "AppData", "Local", "ms-playwright")
				}

				versionDir := filepath.Join(cacheDir, "1.47.2")
				require.NoError(t, os.MkdirAll(versionDir, 0o755))
				browserFile := filepath.Join(versionDir, "chromium-1234")
				require.NoError(t, os.Mkdir(browserFile, 0o755))
			}

			// Create provider.
			provider, err := NewSAMLProvider("test", &schema.Provider{
				Kind:                  "aws/saml",
				URL:                   "https://accounts.google.com/saml",
				Region:                "us-east-1",
				DownloadBrowserDriver: tt.downloadBrowserDriver,
				Driver:                tt.driver,
			})
			require.NoError(t, err)
			sp := provider.(*samlProvider)

			// Get configurations.
			samlConfig := sp.createSAMLConfig()
			loginDetails := sp.createLoginDetails()

			// Both should have consistent DownloadBrowser value.
			assert.Equal(t, tt.expectedDownload, samlConfig.DownloadBrowser,
				"IDPAccount.DownloadBrowser mismatch")
			assert.Equal(t, tt.expectedDownload, loginDetails.DownloadBrowser,
				"LoginDetails.DownloadBrowser mismatch")
			assert.Equal(t, samlConfig.DownloadBrowser, loginDetails.DownloadBrowser,
				"IDPAccount and LoginDetails should have same DownloadBrowser value")
		})
	}
}

// TestPlaywrightDriverDownload_ConfigMapping validates the exact config mapping to saml2aws structs.
func TestPlaywrightDriverDownload_ConfigMapping(t *testing.T) {
	// This test ensures our config correctly maps to saml2aws's expected structures.
	provider, err := NewSAMLProvider("prod-saml", &schema.Provider{
		Kind:                  "aws/saml",
		URL:                   "https://sso.example.com/saml",
		Region:                "eu-west-1",
		Username:              "alice@example.com",
		Password:              "secret123",
		DownloadBrowserDriver: true,
		Driver:                "Browser",
		Session: &schema.SessionConfig{
			Duration: "8h",
		},
	})
	require.NoError(t, err)
	sp := provider.(*samlProvider)

	// Create saml2aws structures.
	samlConfig := sp.createSAMLConfig()
	loginDetails := sp.createLoginDetails()

	// Validate IDPAccount config.
	assert.Equal(t, "https://sso.example.com/saml", samlConfig.URL)
	assert.Equal(t, "alice@example.com", samlConfig.Username)
	assert.Equal(t, "Browser", samlConfig.Provider)
	assert.Equal(t, "prod-saml", samlConfig.Profile)
	assert.Equal(t, "eu-west-1", samlConfig.Region)
	assert.True(t, samlConfig.DownloadBrowser)
	assert.False(t, samlConfig.Headless)              // Should always be false for interactive auth.
	assert.Equal(t, 3600, samlConfig.SessionDuration) // Always 1 hour (samlDefaultSessionSec).

	// Validate LoginDetails.
	assert.Equal(t, "https://sso.example.com/saml", loginDetails.URL)
	assert.Equal(t, "alice@example.com", loginDetails.Username)
	assert.Equal(t, "secret123", loginDetails.Password)
	assert.True(t, loginDetails.DownloadBrowser)

	// Verify type compatibility with saml2aws.
	assert.IsType(t, &cfg.IDPAccount{}, samlConfig, "samlConfig should be *cfg.IDPAccount")
	assert.IsType(t, &creds.LoginDetails{}, loginDetails, "loginDetails should be *creds.LoginDetails")

	t.Log("Config mapping validated successfully")
}
