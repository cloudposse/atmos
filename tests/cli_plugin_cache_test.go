package tests

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tests/testhelpers"
)

// TestTerraformPluginCache verifies that Terraform provider caching works correctly.
// It runs terraform init on two components that use the same provider and verifies
// that the provider is cached in the XDG cache directory.
func TestTerraformPluginCache(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
	}

	// Skip if there's a skip reason.
	if skipReason != "" {
		t.Skipf("Skipping test: %s", skipReason)
	}

	// Create a temporary cache directory for this test.
	tmpCacheDir := t.TempDir()

	// Clear Atmos config env vars.
	err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	require.NoError(t, err)
	err = os.Unsetenv("ATMOS_BASE_PATH")
	require.NoError(t, err)

	// Change to the plugin-cache test fixture.
	workDir := "fixtures/scenarios/plugin-cache"
	t.Chdir(workDir)

	// Clean up any leftover .terraform directories from previous test runs.
	cleanupTerraformDirs(t)

	// Environment variables for this test.
	envVars := map[string]string{
		"XDG_CACHE_HOME": tmpCacheDir,
	}

	// Run terraform init on component-a.
	t.Log("Running terraform init on component-a...")
	runTerraformInitWithEnv(t, "component-a", envVars)

	// Verify the plugin cache directory was created.
	pluginCacheDir := filepath.Join(tmpCacheDir, "atmos", "terraform", "plugins")
	require.DirExists(t, pluginCacheDir, "Plugin cache directory should be created")

	// Verify the null provider is in the cache.
	// The structure is: registry.terraform.io/hashicorp/null/<version>/<os>_<arch>/
	nullProviderDir := filepath.Join(pluginCacheDir, "registry.terraform.io", "hashicorp", "null")
	require.DirExists(t, nullProviderDir, "Null provider should be cached")

	// Count provider files in cache before second init.
	cacheFilesBefore := countFilesInDir(t, pluginCacheDir)
	t.Logf("Cache files after first init: %d", cacheFilesBefore)

	// Run terraform init on component-b (uses same provider).
	t.Log("Running terraform init on component-b...")
	runTerraformInitWithEnv(t, "component-b", envVars)

	// Verify cache was reused (no new downloads).
	cacheFilesAfter := countFilesInDir(t, pluginCacheDir)
	t.Logf("Cache files after second init: %d", cacheFilesAfter)

	// The number of files should be the same - the provider wasn't downloaded again.
	assert.Equal(t, cacheFilesBefore, cacheFilesAfter,
		"Cache should be reused for second component - no new files should be added")

	// Clean up terraform directories.
	runTerraformCleanForceWithEnv(t, envVars)
}

// TestTerraformPluginCacheClean verifies that `terraform clean --cache` works correctly.
func TestTerraformPluginCacheClean(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
	}

	// Skip if there's a skip reason.
	if skipReason != "" {
		t.Skipf("Skipping test: %s", skipReason)
	}

	// Create a temporary cache directory for this test.
	tmpCacheDir := t.TempDir()

	// Clear Atmos config env vars.
	err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	require.NoError(t, err)
	err = os.Unsetenv("ATMOS_BASE_PATH")
	require.NoError(t, err)

	// Change to the plugin-cache test fixture.
	workDir := "fixtures/scenarios/plugin-cache"
	t.Chdir(workDir)

	// Clean up any leftover .terraform directories from previous test runs.
	cleanupTerraformDirs(t)

	// Environment variables for this test.
	envVars := map[string]string{
		"XDG_CACHE_HOME": tmpCacheDir,
	}

	// Run terraform init to populate the cache.
	t.Log("Running terraform init to populate cache...")
	runTerraformInitWithEnv(t, "component-a", envVars)

	// Verify the cache directory exists.
	pluginCacheDir := filepath.Join(tmpCacheDir, "atmos", "terraform", "plugins")
	require.DirExists(t, pluginCacheDir, "Plugin cache directory should exist")

	// Run terraform clean --cache --force.
	t.Log("Running terraform clean --cache --force...")
	cmd := atmosRunner.Command("terraform", "clean", "--cache", "--force")
	for k, v := range envVars {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	t.Logf("Clean --cache output:\n%s", stdout.String())
	if err != nil {
		t.Logf("Clean --cache stderr:\n%s", stderr.String())
	}
	require.NoError(t, err, "terraform clean --cache should succeed")

	// Verify the cache directory was deleted.
	_, err = os.Stat(pluginCacheDir)
	assert.True(t, os.IsNotExist(err), "Plugin cache directory should be deleted after clean --cache")

	// Clean up terraform directories.
	runTerraformCleanForceWithEnv(t, envVars)
}

// TestTerraformPluginCacheDisabled verifies that plugin cache can be disabled.
func TestTerraformPluginCacheDisabled(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
	}

	// Skip if there's a skip reason.
	if skipReason != "" {
		t.Skipf("Skipping test: %s", skipReason)
	}

	// Create a temporary cache directory for this test.
	tmpCacheDir := t.TempDir()

	// Clear Atmos config env vars.
	err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	require.NoError(t, err)
	err = os.Unsetenv("ATMOS_BASE_PATH")
	require.NoError(t, err)

	// Change to the plugin-cache test fixture.
	workDir := "fixtures/scenarios/plugin-cache"
	t.Chdir(workDir)

	// Clean up any leftover .terraform directories from previous test runs.
	cleanupTerraformDirs(t)

	// Environment variables for this test - cache disabled.
	envVars := map[string]string{
		"XDG_CACHE_HOME": tmpCacheDir,
		"ATMOS_COMPONENTS_TERRAFORM_PLUGIN_CACHE": "false",
	}

	// Run terraform init.
	t.Log("Running terraform init with cache disabled...")
	runTerraformInitWithEnv(t, "component-a", envVars)

	// Verify the plugin cache directory was NOT created (cache disabled).
	pluginCacheDir := filepath.Join(tmpCacheDir, "atmos", "terraform", "plugins")
	_, err = os.Stat(pluginCacheDir)
	assert.True(t, os.IsNotExist(err),
		"Plugin cache directory should NOT be created when cache is disabled")

	// Clean up terraform directories.
	runTerraformCleanForceWithEnv(t, envVars)
}

// TestTerraformPluginCacheUserOverride verifies that user's TF_PLUGIN_CACHE_DIR takes precedence.
func TestTerraformPluginCacheUserOverride(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
	}

	// Skip if there's a skip reason.
	if skipReason != "" {
		t.Skipf("Skipping test: %s", skipReason)
	}

	// Create temporary directories.
	tmpCacheDir := t.TempDir()
	userCacheDir := t.TempDir()

	// Clear Atmos config env vars.
	err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	require.NoError(t, err)
	err = os.Unsetenv("ATMOS_BASE_PATH")
	require.NoError(t, err)

	// Change to the plugin-cache test fixture.
	workDir := "fixtures/scenarios/plugin-cache"
	t.Chdir(workDir)

	// Clean up any leftover .terraform directories from previous test runs.
	cleanupTerraformDirs(t)

	// Environment variables - user's TF_PLUGIN_CACHE_DIR takes precedence.
	envVars := map[string]string{
		"XDG_CACHE_HOME":      tmpCacheDir,
		"TF_PLUGIN_CACHE_DIR": userCacheDir,
	}

	// Run terraform init.
	t.Log("Running terraform init with user TF_PLUGIN_CACHE_DIR...")
	runTerraformInitWithEnv(t, "component-a", envVars)

	// Verify the Atmos XDG cache directory was NOT used.
	atmosCacheDir := filepath.Join(tmpCacheDir, "atmos", "terraform", "plugins")
	_, err = os.Stat(atmosCacheDir)
	assert.True(t, os.IsNotExist(err),
		"Atmos cache directory should NOT be used when user sets TF_PLUGIN_CACHE_DIR")

	// Verify the user's cache directory was used.
	nullProviderDir := filepath.Join(userCacheDir, "registry.terraform.io", "hashicorp", "null")
	require.DirExists(t, nullProviderDir, "User's cache directory should be used")

	// Clean up terraform directories.
	runTerraformCleanForceWithEnv(t, envVars)
}

// runTerraformInitWithEnv runs terraform init for a component with custom env vars.
// Uses the "test" stack from the plugin-cache fixture.
func runTerraformInitWithEnv(t *testing.T, component string, envVars map[string]string) {
	t.Helper()
	cmd := atmosRunner.Command("terraform", "init", component, "-s", "test")

	// Add custom env vars to the command.
	for k, v := range envVars {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	// Log env vars for debugging.
	t.Logf("Environment variables being set: %v", envVars)
	for _, e := range cmd.Env {
		if len(e) > 0 && (e[0] == 'X' || e[0] == 'T' || e[0] == 'A' || len(e) > 3 && e[:3] == "TF_") {
			t.Logf("  CMD ENV: %s", e)
		}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	t.Logf("Terraform init stdout:\n%s", stdout.String())
	t.Logf("Terraform init stderr:\n%s", stderr.String())
	if err != nil {
		t.Fatalf("Failed to run terraform init %s -s test: %v", component, err)
	}
}

// runTerraformCleanForceWithEnv runs terraform clean --force with custom env vars.
func runTerraformCleanForceWithEnv(t *testing.T, envVars map[string]string) {
	t.Helper()
	cmd := atmosRunner.Command("terraform", "clean", "--force")
	for k, v := range envVars {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	_ = cmd.Run() // Ignore errors - cleanup is best effort.
}

// countFilesInDir counts all files recursively in a directory.
func countFilesInDir(t *testing.T, dir string) int {
	t.Helper()
	count := 0
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			count++
		}
		return nil
	})
	if err != nil {
		t.Logf("Warning: error walking directory %s: %v", dir, err)
	}
	return count
}

// cleanupTerraformDirs removes .terraform directories, lock files, and tfvars files
// from the test fixture components. This ensures Terraform downloads providers fresh.
func cleanupTerraformDirs(t *testing.T) {
	t.Helper()
	componentsDir := "components/terraform"
	for _, comp := range []string{"component-a", "component-b"} {
		compDir := filepath.Join(componentsDir, comp)
		_ = os.RemoveAll(filepath.Join(compDir, ".terraform"))
		_ = os.RemoveAll(filepath.Join(compDir, ".terraform.lock.hcl"))
		_ = os.RemoveAll(filepath.Join(compDir, "test-"+comp+".terraform.tfvars.json"))
	}
}
