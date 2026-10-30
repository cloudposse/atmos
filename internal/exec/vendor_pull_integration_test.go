package exec

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring/install"
	"github.com/cloudposse/atmos/tests"
	"github.com/cloudposse/atmos/tests/testhelpers/gitconfigenv"
)

// TestVendorPullBasicExecution tests basic vendor pull command execution.
// It verifies the vendor2 manifest clones and filters a real, isolated Git repository.
func TestVendorPullBasicExecution(t *testing.T) {
	tests.RequireExecutable(t, "git", "basic vendor pull integration")
	expected := map[string]string{
		"main.tf":         "# local Git component\n",
		"defaults.tfvars": "enabled = true\n",
		"README.md":       "# Vendored fixture\n",
	}
	stacksPath := vendorPullBasicSandbox(t, expected)
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)
	// Fail immediately if the test ever regresses to a network Git transport.
	t.Setenv("GIT_ALLOW_PROTOCOL", "file")
	t.Chdir(stacksPath)

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
	cmd.PersistentFlags().StringP("type", "t", "terraform", "The type of the vendor (terraform or helmfile).")
	cmd.PersistentFlags().Bool("dry-run", false, "Simulate pulling the latest version of the specified component from the remote repository without making any changes.")
	cmd.PersistentFlags().String("tags", "", "Only vendor the components that have the specified tags")
	cmd.PersistentFlags().Bool("everything", false, "Vendor all components")

	plan, err := PlanVendorPull(cmd, nil)
	require.NoError(t, err)
	require.Len(t, plan.Packages, 1)
	assert.Equal(t, install.PkgTypeRemote, plan.Packages[0].PkgType(), "exercise Git fetch rather than local-file copying")

	// Execute the real command and verify only the selected component's included files.
	require.NoError(t, cmd.RunE(cmd, nil), "'atmos vendor pull' command should execute without error")
	target := filepath.Join(stacksPath, "components", "terraform", "github", "stargazers", "main")
	for name, want := range expected {
		contents, readErr := os.ReadFile(filepath.Join(target, name))
		require.NoError(t, readErr)
		assert.Equal(t, want, string(contents), "vendored %s", name)
	}
	assert.NoFileExists(t, filepath.Join(target, "excluded.txt"))
	assert.NoFileExists(t, filepath.Join(target, "outside.tf"))
}

// vendorPullBasicSandbox preserves the vendor2 manifest's subdirectory, version, and include patterns.
func vendorPullBasicSandbox(t *testing.T, expected map[string]string) string {
	t.Helper()
	repository := t.TempDir()
	repo, err := git.PlainInitWithOptions(repository, &git.PlainInitOptions{InitOptions: git.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")}})
	require.NoError(t, err)
	component := filepath.Join(repository, "examples", "demo-library", "github", "stargazers")
	require.NoError(t, os.MkdirAll(component, 0o755))
	for name, contents := range expected {
		require.NoError(t, os.WriteFile(filepath.Join(component, name), []byte(contents), 0o644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(component, "excluded.txt"), []byte("excluded by include patterns\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repository, "outside.tf"), []byte("outside selected subdirectory\n"), 0o644))
	tree, err := repo.Worktree()
	require.NoError(t, err)
	require.NoError(t, tree.AddWithOptions(&git.AddOptions{All: true}))
	_, err = tree.Commit("local vendor fixture", &git.CommitOptions{Author: &object.Signature{
		Name: "Atmos Test", Email: "test@example.com", When: time.Unix(1, 0),
	}})
	require.NoError(t, err)

	// Normalize Windows drive paths and URL-escape spaces before passing the mirror URL to Git.
	sourcePath := filepath.ToSlash(repository)
	if !strings.HasPrefix(sourcePath, "/") {
		sourcePath = "/" + sourcePath
	}
	source := (&url.URL{Scheme: "file", Path: sourcePath}).String()
	project := t.TempDir()
	for _, name := range []string{"atmos.yaml", "vendor.yaml"} {
		contents, readErr := os.ReadFile(filepath.Join("..", "..", "tests", "fixtures", "scenarios", "vendor2", name))
		require.NoError(t, readErr)
		require.NoError(t, os.WriteFile(filepath.Join(project, name), contents, 0o644))
	}
	// Keep Git's checkout independent of developer settings and Windows newline conversion.
	gitConfig := filepath.Join(t.TempDir(), "gitconfig")
	require.NoError(t, os.WriteFile(gitConfig, []byte("[core]\n\tautocrlf = false\n"), 0o644))
	t.Setenv("GIT_CONFIG_GLOBAL", gitConfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	// Keep the original GitHub-style manifest while redirecting only its Git transport locally.
	overrides := map[string]string{}
	gitconfigenv.Append(overrides, os.Environ(), gitconfigenv.GitConfigEntry{
		Key: "url." + source + ".insteadOf", Value: "https://github.com/cloudposse/atmos.git",
	})
	for key, value := range overrides {
		t.Setenv(key, value)
	}
	for _, name := range []string{"ATMOS_GITHUB_CLI", "ATMOS_GITHUB_TOKEN", "ATMOS_PRO_GITHUB_TOKEN", "GITHUB_TOKEN"} {
		t.Setenv(name, "")
	}
	return project
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

	// Run against a private copy of the scenario rather than the shared, git-tracked fixture
	// directly: tests/test-cases/vendor-test.yaml's "atmos_vendor_pull" acceptance test targets
	// the same directory, and on Windows CI the "tests" and "internal/exec" packages run
	// concurrently (internal/ci/acceptance/run.go's runWindowsShard), so both were racing to
	// write/clean up the same component paths.
	workDir := vendorPullWorkflowSandbox(t, "../../tests/fixtures/scenarios/vendor")
	t.Chdir(workDir)

	// Set up vendor pull command with global flags.
	cmd := newTestCommandWithGlobalFlags("pull")

	flags := cmd.Flags()
	flags.String("component", "", "")
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

	// No manual cleanup needed: workDir is a t.TempDir() copy, removed automatically
	// (along with everything vendor pull wrote under it) when the test ends.

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

// vendorPullWorkflowSandbox copies the vendor scenario's config (atmos.yaml, vendor.yaml, and the
// imported vendor/vendor2.yaml) from srcDir into a private t.TempDir(), rewriting vendor.yaml's two
// local-filesystem source references to absolute paths. Those sources are resolved relative to
// wherever vendor.yaml physically lives on disk (internal/exec/vendor_utils.go's
// vendorConfigFilePath), so copying the file verbatim to an arbitrary temp directory would break
// them; an already-absolute source path is instead returned as-is by pkg/utils.JoinPath. Returns
// the sandbox directory to run the test from.
func vendorPullWorkflowSandbox(t *testing.T, srcDir string) string {
	t.Helper()

	mockComponentPath, err := filepath.Abs(filepath.Join(srcDir, "..", "..", "components", "terraform", "mock"))
	require.NoError(t, err, "Failed to resolve absolute path to the shared mock component fixture")
	// Embed as forward slashes: on Windows filepath.Abs returns backslashes, and a raw
	// backslash inside a double-quoted YAML string is parsed as an escape sequence
	// ("yaml: line N: found unknown escape character"). Forward slashes parse fine in YAML
	// and are accepted by Go's path/filesystem functions on Windows too.
	mockComponentURI := filepath.ToSlash(mockComponentPath)

	dstDir := t.TempDir()

	atmosYAML, err := os.ReadFile(filepath.Join(srcDir, "atmos.yaml"))
	require.NoError(t, err, "Failed to read atmos.yaml fixture")
	require.NoError(t, os.WriteFile(filepath.Join(dstDir, "atmos.yaml"), atmosYAML, 0o644), "Failed to write atmos.yaml copy")

	vendorYAML, err := os.ReadFile(filepath.Join(srcDir, "vendor.yaml"))
	require.NoError(t, err, "Failed to read vendor.yaml fixture")
	rewritten := strings.ReplaceAll(string(vendorYAML), "../../../fixtures/components/terraform/mock", mockComponentURI)
	require.NoError(t, os.WriteFile(filepath.Join(dstDir, "vendor.yaml"), []byte(rewritten), 0o644), "Failed to write vendor.yaml copy")

	vendor2YAML, err := os.ReadFile(filepath.Join(srcDir, "vendor", "vendor2.yaml"))
	require.NoError(t, err, "Failed to read vendor/vendor2.yaml fixture")
	require.NoError(t, os.MkdirAll(filepath.Join(dstDir, "vendor"), 0o755), "Failed to create vendor subdirectory")
	require.NoError(t, os.WriteFile(filepath.Join(dstDir, "vendor", "vendor2.yaml"), vendor2YAML, 0o644), "Failed to write vendor2.yaml copy")

	return dstDir
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
	t.Cleanup(func() { _ = os.Remove("vendor.lock.yaml") })

	// Set up command with global flags.
	cmd := newTestCommandWithGlobalFlags("pull")

	flags := cmd.Flags()
	flags.String("component", "s3-bucket", "")
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
