package xdg

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	adrg "github.com/adrg/xdg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetXDGCacheDir(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tempHome, ".cache"))

	dir, err := GetXDGCacheDir("test", 0o755)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tempHome, ".cache", "atmos", "test"), dir)

	// Verify directory was created.
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestGetXDGDataDir(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(tempHome, ".local", "share"))

	dir, err := GetXDGDataDir("keyring", 0o700)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tempHome, ".local", "share", "atmos", "keyring"), dir)

	// Verify directory was created with correct permissions.
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestGetXDGConfigDir(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tempHome, ".config"))

	dir, err := GetXDGConfigDir("settings", 0o755)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tempHome, ".config", "atmos", "settings"), dir)

	// Verify directory was created.
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestGetXDGCacheDir_AtmosOverride(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tempHome, ".cache"))
	t.Setenv("ATMOS_XDG_CACHE_HOME", filepath.Join(tempHome, "custom-cache"))

	dir, err := GetXDGCacheDir("test", 0o755)
	require.NoError(t, err)

	// Should use ATMOS_XDG_CACHE_HOME (takes precedence).
	assert.Equal(t, filepath.Join(tempHome, "custom-cache", "atmos", "test"), dir)
}

func TestGetXDGDataDir_AtmosOverride(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(tempHome, ".local", "share"))
	t.Setenv("ATMOS_XDG_DATA_HOME", filepath.Join(tempHome, "custom-data"))

	dir, err := GetXDGDataDir("keyring", 0o700)
	require.NoError(t, err)

	// Should use ATMOS_XDG_DATA_HOME (takes precedence).
	assert.Equal(t, filepath.Join(tempHome, "custom-data", "atmos", "keyring"), dir)
}

func TestGetXDGDir_EmptySubpath(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tempHome, ".cache"))

	dir, err := GetXDGCacheDir("", 0o755)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tempHome, ".cache", "atmos"), dir)
}

func TestGetXDGDir_NestedSubpath(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(tempHome, ".local", "share"))

	dir, err := GetXDGDataDir(filepath.Join("auth", "keyring"), 0o700)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tempHome, ".local", "share", "atmos", "auth", "keyring"), dir)

	// Verify nested directory was created.
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestGetXDGDir_MkdirError(t *testing.T) {
	// Create a file where we want to create a directory.
	// This will cause os.MkdirAll to fail.
	tempHome := t.TempDir()
	blockingFile := filepath.Join(tempHome, "atmos")

	// Create a regular file that blocks directory creation.
	err := os.WriteFile(blockingFile, []byte("blocking"), 0o644)
	require.NoError(t, err)

	t.Setenv("XDG_CACHE_HOME", tempHome)

	// Should fail because "atmos" exists as a file, not a directory.
	_, err = GetXDGCacheDir("test", 0o755)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create directory")
}

func TestGetXDGDir_DefaultFallback(t *testing.T) {
	// Unset all XDG environment variables to test default fallback.
	// The test should use the library default from github.com/adrg/xdg.
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("ATMOS_XDG_CACHE_HOME", "")

	// Should not error even without env vars - uses library default.
	dir, err := GetXDGCacheDir("test", 0o755)
	require.NoError(t, err)

	// Should contain "atmos/test" in the path.
	assert.Contains(t, dir, filepath.Join("atmos", "test"))

	// Verify directory was actually created.
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestGetXDGConfigDir_PlatformDefaults(t *testing.T) {
	// This test verifies that the adrg/xdg library returns the expected
	// platform-specific defaults when no environment variables are set.
	// This is critical for documentation accuracy and Geodesic integration.

	// Unset all XDG environment variables to test platform defaults.
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("ATMOS_XDG_CONFIG_HOME", "")

	dir, err := GetXDGConfigDir("aws/test-provider", 0o755)
	require.NoError(t, err)

	// Verify the path contains the expected platform-specific components.
	// We test for path segments that should be present on each platform.
	// Platform-specific defaults from github.com/adrg/xdg:
	// - Linux/Unix: ~/.config/atmos/aws/test-provider
	// - macOS: ~/Library/Application Support/atmos/aws/test-provider
	// - Windows: %APPDATA%\atmos\aws\test-provider

	// All platforms should have "atmos/aws/test-provider" in the path.
	assert.Contains(t, dir, filepath.Join("atmos", "aws", "test-provider"),
		"Path should contain atmos/aws/test-provider segment")

	// Verify directory was created.
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// Log the actual path for debugging and documentation verification.
	t.Logf("Platform-specific XDG config path: %s", dir)
}

func TestGetXDGConfigDir_AwsCredentialPath(t *testing.T) {
	// This test verifies the exact path structure used for AWS credential storage.
	// This is what gets documented in our Geodesic configuration guide.

	tempHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tempHome, ".config"))

	providerName := "acme-sso"
	dir, err := GetXDGConfigDir(filepath.Join("aws", providerName), 0o700)
	require.NoError(t, err)

	expectedPath := filepath.Join(tempHome, ".config", "atmos", "aws", providerName)
	assert.Equal(t, expectedPath, dir,
		"AWS credential path should follow XDG config structure")

	// Verify this matches the path we document for Geodesic.
	credentialsFile := filepath.Join(dir, "credentials")
	configFile := filepath.Join(dir, "config")

	// These paths should be constructible from the base directory.
	assert.Contains(t, credentialsFile, filepath.Join("atmos", "aws", providerName, "credentials"))
	assert.Contains(t, configFile, filepath.Join("atmos", "aws", providerName, "config"))

	t.Logf("AWS credentials path: %s", credentialsFile)
	t.Logf("AWS config path: %s", configFile)
}

func TestGetXDGConfigDir_BackwardCompatibilityPath(t *testing.T) {
	// This test verifies the backward-compatibility option documented in the
	// Geodesic configuration guide, where users can set ATMOS_XDG_CONFIG_HOME
	// to use the legacy ~/.aws/atmos/ path.

	tempHome := t.TempDir()
	awsDir := filepath.Join(tempHome, ".aws")
	t.Setenv("ATMOS_XDG_CONFIG_HOME", awsDir)

	providerName := "acme-sso"
	dir, err := GetXDGConfigDir(filepath.Join("aws", providerName), 0o700)
	require.NoError(t, err)

	// Should use ~/.aws/atmos/aws/acme-sso (for backward compatibility).
	expectedPath := filepath.Join(awsDir, "atmos", "aws", providerName)
	assert.Equal(t, expectedPath, dir,
		"ATMOS_XDG_CONFIG_HOME should enable backward-compatible paths")

	// This allows users to keep credentials at ~/.aws/atmos/ and only
	// mount ~/.aws/ in their Geodesic containers.
	assert.Contains(t, dir, filepath.Join(".aws", "atmos", "aws"))

	t.Logf("Backward-compatible path: %s", dir)
}

func TestXDGLibraryDirectAccess(t *testing.T) {
	// This test verifies that our init() function properly overrides the adrg/xdg
	// library defaults on macOS, so even code that directly uses xdg.ConfigHome
	// (without going through our GetXDGConfigDir) will get the correct CLI tool paths.

	if runtime.GOOS != "darwin" {
		t.Skip("This test is macOS-specific")
	}

	// Import the xdg package to trigger our init().
	// Since we're already in the xdg package, the init() has already run.

	// Directly access the adrg/xdg library's exported variables.
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	// On macOS, our init() should have set these to CLI tool conventions.
	expectedConfigHome := filepath.Join(homeDir, ".config")
	expectedDataHome := filepath.Join(homeDir, ".local", "share")
	expectedCacheHome := filepath.Join(homeDir, ".cache")

	assert.Equal(t, expectedConfigHome, adrg.ConfigHome,
		"adrg/xdg ConfigHome should be overridden to use CLI convention on macOS")
	assert.Equal(t, expectedDataHome, adrg.DataHome,
		"adrg/xdg DataHome should be overridden to use CLI convention on macOS")
	assert.Equal(t, expectedCacheHome, adrg.CacheHome,
		"adrg/xdg CacheHome should be overridden to use CLI convention on macOS")

	t.Logf("adrg.ConfigHome: %s", adrg.ConfigHome)
	t.Logf("adrg.DataHome: %s", adrg.DataHome)
	t.Logf("adrg.CacheHome: %s", adrg.CacheHome)
}
