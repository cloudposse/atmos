package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/tests"
)

// testExecuteVendorPullCmd is a test helper that wraps ExecuteVendorPullCmd for old-style tests.
// It parses flags from cmd and converts them to StandardOptions.
func testExecuteVendorPullCmd(cmd *cobra.Command, args []string) error {
	cmdFlags := cmd.Flags()

	// Parse flags from cmd
	component, _ := cmdFlags.GetString("component")
	stack, _ := cmdFlags.GetString("stack")
	typ, _ := cmdFlags.GetString("type")
	dryRun, _ := cmdFlags.GetBool("dry-run")
	tagsStr, _ := cmdFlags.GetString("tags")
	everything, _ := cmdFlags.GetBool("everything")

	// Create StandardOptions
	opts := &flags.StandardOptions{
		Component:  component,
		Stack:      stack,
		Type:       typ,
		DryRun:     dryRun,
		Tags:       tagsStr,
		Everything: everything,
	}

	return ExecuteVendorPullCmd(opts)
}

// testExecuteVendorPullCommand is a test helper for ExecuteVendorPullCommand.
//
//nolint:unparam // args parameter kept for Cobra RunE signature compatibility
func testExecuteVendorPullCommand(cmd *cobra.Command, args []string) error {
	cmdFlags := cmd.Flags()

	// Parse flags from cmd
	component, _ := cmdFlags.GetString("component")
	stack, _ := cmdFlags.GetString("stack")
	typ, _ := cmdFlags.GetString("type")
	dryRun, _ := cmdFlags.GetBool("dry-run")
	tagsStr, _ := cmdFlags.GetString("tags")
	everything, _ := cmdFlags.GetBool("everything")

	// Create StandardOptions
	var tagsList []string
	if tagsStr != "" {
		tagsList = strings.Split(tagsStr, ",")
	}

	opts := &flags.StandardOptions{
		Component:  component,
		Stack:      stack,
		Type:       typ,
		DryRun:     dryRun,
		Tags:       tagsStr,
		Everything: everything,
	}

	// Set default for 'everything' if no specific flags are provided
	if !opts.Everything && opts.Component == "" && opts.Stack == "" && len(tagsList) == 0 {
		opts.Everything = true
	}

	return ExecuteVendorPullCommand(opts)
}

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

	cmd := &cobra.Command{
		Use:                "pull",
		Short:              "Pull the latest vendor configurations or dependencies",
		Long:               "Pull and update vendor-specific configurations or dependencies to ensure the project has the latest required resources.",
		FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
		Args:               cobra.NoArgs,
		RunE:               testExecuteVendorPullCmd,
	}

	cmd.DisableFlagParsing = false
	cmd.PersistentFlags().StringP("component", "c", "", "Only vendor the specified component")
	cmd.PersistentFlags().StringP("stack", "s", "", "Only vendor the specified stack")
	cmd.PersistentFlags().StringP("type", "t", "terraform", "The type of the vendor (terraform or helmfile).")
	cmd.PersistentFlags().Bool("dry-run", false, "Simulate pulling the latest version of the specified component from the remote repository without making any changes.")
	cmd.PersistentFlags().String("tags", "", "Only vendor the components that have the specified tags")
	cmd.PersistentFlags().Bool("everything", false, "Vendor all components")
	cmd.PersistentFlags().String("base-path", "", "Base path for Atmos project")
	cmd.PersistentFlags().StringSlice("config", []string{}, "Paths to configuration file")
	cmd.PersistentFlags().StringSlice("config-path", []string{}, "Path to configuration directory")

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

	// Set up vendor pull command.
	cmd := &cobra.Command{}
	cmd.PersistentFlags().String("base-path", "", "Base path for Atmos project")
	cmd.PersistentFlags().StringSlice("config", []string{}, "Paths to configuration file")
	cmd.PersistentFlags().StringSlice("config-path", []string{}, "Path to configuration directory")

	flags := cmd.Flags()
	flags.String("component", "", "")
	flags.String("stack", "", "")
	flags.String("tags", "", "")
	flags.Bool("dry-run", false, "")
	flags.Bool("everything", false, "")

	// Test 1: Execute vendor pull and verify files are created.
	err := testExecuteVendorPullCommand(cmd, []string{})
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

	err = testExecuteVendorPullCommand(cmd, []string{})
	require.NoError(t, err, "Dry run should execute without error")

	// Test 3: Tag filtering should work.
	err = flags.Set("dry-run", "false")
	require.NoError(t, err, "Failed to reset dry-run flag")

	err = flags.Set("tags", "demo")
	require.NoError(t, err, "Failed to set tags flag")

	err = testExecuteVendorPullCommand(cmd, []string{})
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

	// Set up command.
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
