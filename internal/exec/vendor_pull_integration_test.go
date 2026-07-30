package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/tests"
)

// TestVendorPullBasicExecution tests basic vendor pull command execution.
// It verifies the command runs without errors using the vendor2 fixture.
func TestVendorPullBasicExecution(t *testing.T) {
	// Skip long tests in short mode (this test takes ~4 seconds due to network I/O and Git operations)
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

	// Create command with global flags (including profile flag).
	cmd := newTestCommandWithGlobalFlags("pull")
	cmd.Short = "Pull the latest vendor configurations or dependencies"
	cmd.Long = "Pull and update vendor-specific configurations or dependencies to ensure the project has the latest required resources."
	cmd.FParseErrWhitelist = struct{ UnknownFlags bool }{UnknownFlags: false}
	cmd.Args = cobra.NoArgs
	cmd.RunE = ExecuteVendorPullCmd

	// Add vendor-specific flags.
	cmd.DisableFlagParsing = false
	cmd.PersistentFlags().StringP("component", "c", "", "Only vendor the specified component")
	cmd.PersistentFlags().StringP("stack", "s", "", "Only vendor the specified stack")
	cmd.PersistentFlags().StringP("type", "t", "terraform", "The type of the vendor (terraform or helmfile).")
	cmd.PersistentFlags().Bool("dry-run", false, "Simulate pulling the latest version of the specified component from the remote repository without making any changes.")
	cmd.PersistentFlags().String("tags", "", "Only vendor the components that have the specified tags")
	cmd.PersistentFlags().Bool("everything", false, "Vendor all components")

	// Execute the command.
	err := cmd.RunE(cmd, []string{})
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
	// Skip long tests in short mode (this test requires network I/O and OCI pulls)
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

	// Set up vendor pull command with global flags.
	cmd := newTestCommandWithGlobalFlags("pull")

	flags := cmd.Flags()
	flags.String("component", "", "")
	flags.String("stack", "", "")
	flags.String("tags", "", "")
	flags.Bool("dry-run", false, "")
	flags.Bool("everything", false, "")

	// Test 1: Execute vendor pull and verify files are created.
	err := ExecuteVendorPullCommand(cmd, []string{})
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
	err = flags.Set("dry-run", "true")
	require.NoError(t, err, "Failed to set dry-run flag")

	err = ExecuteVendorPullCommand(cmd, []string{})
	require.NoError(t, err, "Dry run should execute without error")

	// Test 3: Tag filtering should work.
	err = flags.Set("dry-run", "false")
	require.NoError(t, err, "Failed to reset dry-run flag")

	err = flags.Set("tags", "demo")
	require.NoError(t, err, "Failed to set tags flag")

	err = ExecuteVendorPullCommand(cmd, []string{})
	require.NoError(t, err, "Tag filtering should execute without error")
}

// vendorBasePathFixture writes an Atmos project where atmos.yaml and vendor.yaml live in a `config`
// subdirectory while `base_path` points at the project root, and returns the project root.
// This is the layout from https://github.com/cloudposse/atmos/issues/2409.
func vendorBasePathFixture(t *testing.T) string {
	t.Helper()

	// Resolve symlinks (macOS maps /var to /private/var) so the paths written into atmos.yaml match
	// the paths the assertions stat.
	tempDir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	configDir := filepath.Join(tempDir, "config")
	require.NoError(t, os.MkdirAll(configDir, 0o755))

	// Vendor from a local directory so the test needs no network access.
	sourceDir := filepath.Join(tempDir, "source", "mock")
	require.NoError(t, os.MkdirAll(sourceDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte("# mock component\n"), 0o644))

	// Forward slashes keep the YAML valid on Windows, where an absolute path contains backslashes.
	atmosYAML := fmt.Sprintf(`base_path: "%s"

vendor:
  base_path: "config/vendor.yaml"

components:
  terraform:
    base_path: "components/terraform"

stacks:
  base_path: "stacks"
  included_paths:
    - "**/*"

logs:
  level: Info
`, filepath.ToSlash(tempDir))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "atmos.yaml"), []byte(atmosYAML), 0o644))

	vendorYAML := fmt.Sprintf(`apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: vendor-base-path
  description: Vendor manifest stored next to atmos.yaml, outside the project root
spec:
  sources:
    - component: "mock"
      source: "%s"
      version: "1.0.0"
      targets:
        - "components/terraform/{{ .Component }}"
      included_paths:
        - "**/*.tf"
`, filepath.ToSlash(sourceDir))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "vendor.yaml"), []byte(vendorYAML), 0o644))

	return tempDir
}

// newVendorPullTestCmd builds a `vendor pull` command with the flags ExecuteVendorPullCommand reads.
func newVendorPullTestCmd() *cobra.Command {
	cmd := newTestCommandWithGlobalFlags("pull")

	flags := cmd.Flags()
	flags.String("component", "", "")
	flags.String("stack", "", "")
	flags.String("tags", "", "")
	flags.Bool("dry-run", false, "")
	flags.Bool("everything", false, "")

	return cmd
}

// TestVendorPullTargetsResolveAgainstBasePath verifies that when Atmos locates vendor.yaml through
// the `vendor.base_path` setting in atmos.yaml, relative `targets` resolve against the Atmos
// `base_path` instead of the directory holding vendor.yaml.
// Regression test for https://github.com/cloudposse/atmos/issues/2409.
func TestVendorPullTargetsResolveAgainstBasePath(t *testing.T) {
	tempDir := vendorBasePathFixture(t)

	t.Chdir(tempDir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", filepath.Join(tempDir, "config"))

	err := ExecuteVendorPullCommand(newVendorPullTestCmd(), []string{})
	require.NoError(t, err, "'atmos vendor pull' should execute without error")

	// The artifact lands under `base_path`, where `components.terraform.base_path` points.
	assert.FileExists(t, filepath.Join(tempDir, "components", "terraform", "mock", "main.tf"),
		"vendored component should be written relative to the Atmos base path")

	// And not next to vendor.yaml, which is what issue #2409 reported.
	assert.NoDirExists(t, filepath.Join(tempDir, "config", "components"),
		"vendored component should not be written next to vendor.yaml")
}

// TestVendorPullTargetsRelativeToManifestWithoutVendorBasePath verifies the negative path: without
// `vendor.base_path` in atmos.yaml the manifest is discovered next to the working directory, so
// relative targets keep resolving against the manifest's own directory.
func TestVendorPullTargetsRelativeToManifestWithoutVendorBasePath(t *testing.T) {
	tempDir := vendorBasePathFixture(t)
	configDir := filepath.Join(tempDir, "config")

	// Drop the `vendor.base_path` setting, leaving vendor.yaml to be discovered in the working
	// directory instead.
	atmosYAML := fmt.Sprintf(`base_path: "%s"

components:
  terraform:
    base_path: "components/terraform"

stacks:
  base_path: "stacks"
  included_paths:
    - "**/*"

logs:
  level: Info
`, filepath.ToSlash(tempDir))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "atmos.yaml"), []byte(atmosYAML), 0o644))

	// Run from the config directory, where vendor.yaml lives.
	t.Chdir(configDir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", configDir)

	err := ExecuteVendorPullCommand(newVendorPullTestCmd(), []string{})
	require.NoError(t, err, "'atmos vendor pull' should execute without error")

	assert.FileExists(t, filepath.Join(configDir, "components", "terraform", "mock", "main.tf"),
		"vendored component should be written relative to the vendor manifest")
	assert.NoDirExists(t, filepath.Join(tempDir, "components"),
		"vendored component should not be written relative to the Atmos base path")
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

	// Set up command with global flags.
	cmd := newTestCommandWithGlobalFlags("pull")

	flags := cmd.Flags()
	flags.String("component", "s3-bucket", "")
	flags.String("stack", "", "")
	flags.String("tags", "", "")
	flags.Bool("dry-run", false, "")
	flags.Bool("everything", false, "")

	// Execute vendor pull command.
	err := ExecuteVendorPullCommand(cmd, []string{})
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
