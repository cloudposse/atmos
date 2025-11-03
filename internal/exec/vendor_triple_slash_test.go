package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tests"
	"github.com/spf13/cobra"
)

// TestVendorPullWithTripleSlashPattern tests the vendor pull command with the triple-slash pattern.
// The pattern indicates cloning from the root of a repository (e.g., github.com/repo.git///?ref=v1.0).
// This pattern was broken after go-getter v1.7.9 due to changes in subdirectory path handling.
func TestVendorPullWithTripleSlashPattern(t *testing.T) {
	// Check for GitHub access with rate limit check.
	rateLimits := tests.RequireGitHubAccess(t)
	if rateLimits != nil && rateLimits.Remaining < 10 {
		t.Skipf("Insufficient GitHub API requests remaining (%d). Test may require ~10 requests", rateLimits.Remaining)
	}

	// Set environment variables using t.Setenv (automatically restores on cleanup).
	t.Setenv("ATMOS_CLI_CONFIG_PATH", "./atmos.yaml")
	t.Setenv("ATMOS_BASE_PATH", ".")
	t.Setenv("ATMOS_LOGS_LEVEL", "Debug")

	// Define the test directory.
	testDir := "../../tests/fixtures/scenarios/vendor-triple-slash"

	// Change to the test directory.
	t.Chdir(testDir)

	// Set up the command.
	cmd := &cobra.Command{}
	cmd.PersistentFlags().String("base-path", "", "Base path for Atmos project")
	cmd.PersistentFlags().StringSlice("config", []string{}, "Paths to configuration file")
	cmd.PersistentFlags().StringSlice("config-path", []string{}, "Path to configuration directory")

	flags := cmd.Flags()
	flags.String("component", "s3-bucket", "")
	flags.String("stack", "", "")
	flags.String("tags", "", "")
	flags.Bool("dry-run", false, "")
	flags.Bool("everything", false, "")

	// Execute vendor pull command.
	err := testExecuteVendorPullCommand(cmd, []string{})
	require.NoError(t, err, "Vendor pull command should execute without error")

	// Check that the target directory was created.
	targetDir := filepath.Join("components", "terraform", "s3-bucket")
	assert.DirExists(t, targetDir, "Target directory should be created")

	// Test that the expected files were pulled from the repository.
	// According to the bug report, these files should be present but are not being pulled.
	expectedFiles := []string{
		// Main terraform files that should match "**/*.tf".
		filepath.Join(targetDir, "main.tf"),
		filepath.Join(targetDir, "outputs.tf"),
		filepath.Join(targetDir, "variables.tf"),
		filepath.Join(targetDir, "versions.tf"),

		// Documentation files that should match "**/README.md".
		filepath.Join(targetDir, "README.md"),

		// License file that should match "**/LICENSE".
		filepath.Join(targetDir, "LICENSE"),

		// Module files that should match "**/modules/**".
		// The terraform-aws-s3-bucket repository has modules subdirectory.
		filepath.Join(targetDir, "modules", "notification", "main.tf"),
		filepath.Join(targetDir, "modules", "notification", "variables.tf"),
		filepath.Join(targetDir, "modules", "notification", "outputs.tf"),
		filepath.Join(targetDir, "modules", "notification", "versions.tf"),
	}

	// Check that files were actually pulled (not just an empty directory).
	// This is the main assertion that should fail based on the bug report.
	for _, file := range expectedFiles {
		assert.FileExists(t, file, "File should be pulled from repository: %s", file)
	}

	// Clean up: Remove the created directory and its contents.
	t.Cleanup(func() {
		if err := os.RemoveAll(targetDir); err != nil {
			t.Logf("Failed to clean up target directory: %v", err)
		}
	})
}

// TestVendorPullWithMultipleVendorFiles tests that vendor pull works correctly.
// It handles the case where there are multiple vendor YAML files in the same directory.
// This could be a potential cause of the issue where the vendor process
// gets confused by multiple configuration files.
func TestVendorPullWithMultipleVendorFiles(t *testing.T) {
	// Check for GitHub access with rate limit check.
	rateLimits := tests.RequireGitHubAccess(t)
	if rateLimits != nil && rateLimits.Remaining < 10 {
		t.Skipf("Insufficient GitHub API requests remaining (%d). Test may require ~10 requests", rateLimits.Remaining)
	}

	// Set environment variables using t.Setenv (automatically restores on cleanup).
	t.Setenv("ATMOS_CLI_CONFIG_PATH", "./atmos.yaml")
	t.Setenv("ATMOS_BASE_PATH", ".")
	t.Setenv("ATMOS_LOGS_LEVEL", "Debug")

	// Define the test directory.
	testDir := "../../tests/fixtures/scenarios/vendor-triple-slash"

	// Change to the test directory.
	t.Chdir(testDir)

	// Verify that multiple vendor files exist in the directory.
	vendorFiles := []string{"vendor.yaml", "vendor-test.yaml"}
	for _, file := range vendorFiles {
		assert.FileExists(t, file, "Vendor file should exist: %s", file)
	}

	// Set up the command.
	cmd := &cobra.Command{}
	cmd.PersistentFlags().String("base-path", "", "Base path for Atmos project")
	cmd.PersistentFlags().StringSlice("config", []string{}, "Paths to configuration file")
	cmd.PersistentFlags().StringSlice("config-path", []string{}, "Path to configuration directory")

	flags := cmd.Flags()
	flags.String("component", "", "")
	flags.String("stack", "", "")
	flags.String("tags", "aws", "")
	flags.Bool("dry-run", false, "")
	flags.Bool("everything", false, "")

	// Execute vendor pull command with tags filter.
	err := testExecuteVendorPullCommand(cmd, []string{})
	require.NoError(t, err, "Vendor pull command should execute without error even with multiple vendor files")

	// Check that the s3-bucket component was pulled (it has the 'aws' tag).
	targetDir := filepath.Join("components", "terraform", "s3-bucket")
	assert.DirExists(t, targetDir, "Target directory for s3-bucket should be created")

	// Verify that at least some files were pulled.
	entries, err := os.ReadDir(targetDir)
	assert.NoError(t, err, "Should be able to read target directory")
	assert.NotEmpty(t, entries, "Target directory should not be empty - files should have been pulled")

	// Clean up: Remove the created directory and its contents.
	t.Cleanup(func() {
		if err := os.RemoveAll(targetDir); err != nil {
			t.Logf("Failed to clean up target directory: %v", err)
		}
	})
}
