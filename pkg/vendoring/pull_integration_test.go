package vendoring

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/tests"
)

// TestVendorPullBasicExecution tests basic vendor pull command execution.
// It verifies the command runs without errors using the vendor2 fixture.
func TestVendorPullBasicExecution(t *testing.T) {
	// Skip long tests in short mode (this test takes ~4 seconds due to network I/O and Git operations).
	tests.SkipIfShort(t)

	// Check for GitHub access with rate limit check.
	rateLimits := tests.RequireGitHubAccess(t)
	if rateLimits != nil && rateLimits.Remaining < 10 {
		t.Skipf("Insufficient GitHub API requests remaining (%d). Test may require ~10 requests.", rateLimits.Remaining)
	}

	stacksPath := "../../tests/fixtures/scenarios/vendor2"

	// Use t.Setenv for automatic cleanup.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	// Initialize atmos config.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err, "Failed to initialize atmos config")

	// Execute Pull with default params (should vendor everything).
	err = Pull(&atmosConfig, &PullParams{
		Everything: true,
	})
	assert.NoError(t, err, "'atmos vendor pull' command should execute without error")
}

// TestVendorPullConfigFileProcessing tests reading and processing vendor config files.
func TestVendorPullConfigFileProcessing(t *testing.T) {
	basePath := "../../tests/fixtures/scenarios/vendor2"
	vendorConfigFile := "vendor.yaml"

	atmosConfig := schema.AtmosConfiguration{
		BasePath: basePath,
	}

	_, _, _, err := ReadAndProcessVendorConfigFile(&atmosConfig, vendorConfigFile, false)
	assert.NoError(t, err, "ReadAndProcessVendorConfigFile should execute without error")
}

// TestVendorPullFullWorkflow tests the complete vendor pull workflow including file verification.
// It verifies that vendor components are correctly pulled from various sources (git, file, OCI).
func TestVendorPullFullWorkflow(t *testing.T) {
	// Skip long tests in short mode (this test requires network I/O and OCI pulls).
	tests.SkipIfShort(t)

	// Check for GitHub access with rate limit check.
	rateLimits := tests.RequireGitHubAccess(t)
	if rateLimits != nil && rateLimits.Remaining < 20 {
		t.Skipf("Insufficient GitHub API requests remaining (%d). Test may require ~20 requests.", rateLimits.Remaining)
	}

	// Check for OCI authentication (GitHub token) for pulling images from ghcr.io.
	tests.RequireOCIAuthentication(t)

	// Change to test fixture directory.
	workDir := "../../tests/fixtures/scenarios/vendor"
	t.Chdir(workDir)

	// Initialize atmos config.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err, "Failed to initialize atmos config")

	// Test 1: Execute vendor pull and verify files are created.
	err = Pull(&atmosConfig, &PullParams{
		Everything: true,
	})
	require.NoError(t, err, "Failed to execute vendor pull command")

	expectedFiles := []string{
		"./components/terraform/github/stargazers/main/main.tf",
		"./components/terraform/github/stargazers/main/outputs.tf",
		"./components/terraform/github/stargazers/main/providers.tf",
		"./components/terraform/github/stargazers/main/variables.tf",
		"./components/terraform/github/stargazers/main/versions.tf",
		"./components/terraform/github/stargazers/main/README.md",
		"./components/terraform/test-components/main/main.tf",
		"./components/terraform/test-components/main/README.md",
		"./components/terraform/weather/main/main.tf",
		"./components/terraform/weather/main/outputs.tf",
		"./components/terraform/weather/main/providers.tf",
		"./components/terraform/weather/main/variables.tf",
		"./components/terraform/weather/main/versions.tf",
		"./components/terraform/weather/main/README.md",
		"./components/terraform/myapp2/main.tf",
		"./components/terraform/myapp2/README.md",
		"./components/terraform/myapp1/main.tf",
		"./components/terraform/myapp1/README.md",
	}

	// Verify all expected files exist.
	for _, file := range expectedFiles {
		assert.FileExists(t, file, "Expected file should exist: %s", file)
	}

	// Clean up vendored files.
	t.Cleanup(func() {
		for _, file := range expectedFiles {
			// Remove individual files and their parent directories.
			dir := filepath.Dir(file)
			os.RemoveAll(dir)
		}
	})

	// Test 2: Dry-run flag should not fail.
	err = Pull(&atmosConfig, &PullParams{
		Everything: true,
		DryRun:     true,
	})
	require.NoError(t, err, "Dry run should execute without error")

	// Test 3: Tag filtering should work.
	err = Pull(&atmosConfig, &PullParams{
		Tags: "demo",
	})
	require.NoError(t, err, "Tag filtering should execute without error")
}

// TestVendorPullTripleSlashNormalization tests end-to-end triple-slash URI normalization.
// This complements the unit tests with integration-level verification.
func TestVendorPullTripleSlashNormalization(t *testing.T) {
	// Check for GitHub access with rate limit check.
	rateLimits := tests.RequireGitHubAccess(t)
	if rateLimits != nil && rateLimits.Remaining < 10 {
		t.Skipf("Insufficient GitHub API requests remaining (%d). Test may require ~10 requests.", rateLimits.Remaining)
	}

	// Use t.Setenv for automatic cleanup.
	t.Setenv("ATMOS_LOGS_LEVEL", "Debug")

	// Change to test directory.
	testDir := "../../tests/fixtures/scenarios/vendor-triple-slash"
	t.Chdir(testDir)

	// Initialize atmos config.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err, "Failed to initialize atmos config")

	// Execute vendor pull command with component filter.
	err = Pull(&atmosConfig, &PullParams{
		Component: "s3-bucket",
	})
	require.NoError(t, err, "Vendor pull command with triple-slash URI should execute without error")

	// Verify target directory was created.
	targetDir := filepath.Join("components", "terraform", "s3-bucket")
	assert.DirExists(t, targetDir, "Target directory should be created")

	// Verify expected files were pulled.
	expectedFiles := []string{
		filepath.Join(targetDir, "main.tf"),
		filepath.Join(targetDir, "outputs.tf"),
		filepath.Join(targetDir, "variables.tf"),
		filepath.Join(targetDir, "versions.tf"),
		filepath.Join(targetDir, "README.md"),
		filepath.Join(targetDir, "LICENSE"),
	}

	for _, file := range expectedFiles {
		assert.FileExists(t, file, "File should be pulled from repository: %s", file)
	}

	// Clean up.
	t.Cleanup(func() {
		os.RemoveAll(targetDir)
	})
}
