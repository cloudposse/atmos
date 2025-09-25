package testhelpers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSandboxCreation(t *testing.T) {
	// Test creating a sandbox environment.
	workdir := "../fixtures/scenarios/env"

	sandbox, err := SetupSandbox(t, workdir)
	require.NoError(t, err, "Failed to setup sandbox")
	require.NotNil(t, sandbox, "Sandbox should not be nil")

	// Ensure cleanup happens.
	defer sandbox.Cleanup()

	// Verify temp directory exists.
	assert.DirExists(t, sandbox.TempDir, "Sandbox temp directory should exist")

	// Verify components were copied.
	assert.DirExists(t, sandbox.ComponentsPath, "Components path should exist")

	// Check that terraform components were copied.
	terraformPath := filepath.Join(sandbox.ComponentsPath, "terraform")
	if _, err := os.Stat(terraformPath); err == nil {
		assert.DirExists(t, terraformPath, "Terraform components should be copied")

		// Verify no terraform artifacts were copied.
		tfLockFile := filepath.Join(terraformPath, "env-example", ".terraform.lock.hcl")
		assert.NoFileExists(t, tfLockFile, ".terraform.lock.hcl should not be copied")

		tfDir := filepath.Join(terraformPath, "env-example", ".terraform")
		assert.NoDirExists(t, tfDir, ".terraform directory should not be copied")
	}

	// Verify environment variables are set correctly.
	envVars := sandbox.GetEnvironmentVariables()
	if terraformPath := envVars["ATMOS_COMPONENTS_TERRAFORM_BASE_PATH"]; terraformPath != "" {
		assert.Contains(t, terraformPath, sandbox.TempDir, "Terraform path should be in sandbox")
	}
}

func TestSandboxEnvironmentVariables(t *testing.T) {
	// Test that sandbox sets correct environment variables.
	workdir := "../fixtures/scenarios/env"

	sandbox, err := SetupSandbox(t, workdir)
	require.NoError(t, err, "Failed to setup sandbox")
	defer sandbox.Cleanup()

	envVars := sandbox.GetEnvironmentVariables()

	// Check terraform base path if terraform components exist.
	if tfPath, exists := envVars["ATMOS_COMPONENTS_TERRAFORM_BASE_PATH"]; exists {
		assert.NotEmpty(t, tfPath, "Terraform base path should not be empty")
		assert.DirExists(t, tfPath, "Terraform base path should exist")
		assert.Contains(t, tfPath, "atmos-sandbox", "Path should contain sandbox identifier")
	}

	// Check helmfile base path if helmfile components exist.
	if helmPath, exists := envVars["ATMOS_COMPONENTS_HELMFILE_BASE_PATH"]; exists {
		assert.NotEmpty(t, helmPath, "Helmfile base path should not be empty")
		assert.DirExists(t, helmPath, "Helmfile base path should exist")
		assert.Contains(t, helmPath, "atmos-sandbox", "Path should contain sandbox identifier")
	}
}

func TestSandboxCleanup(t *testing.T) {
	// Test that sandbox cleanup removes temporary files.
	workdir := "../fixtures/scenarios/env"

	sandbox, err := SetupSandbox(t, workdir)
	require.NoError(t, err, "Failed to setup sandbox")

	tempDir := sandbox.TempDir
	require.DirExists(t, tempDir, "Temp directory should exist before cleanup")

	// Clean up the sandbox.
	sandbox.Cleanup()

	// Verify temp directory is removed.
	assert.NoDirExists(t, tempDir, "Temp directory should be removed after cleanup")
}

func TestSandboxWithNonExistentWorkdir(t *testing.T) {
	// Test sandbox setup with non-existent workdir.
	workdir := "non-existent-directory"

	sandbox, err := SetupSandbox(t, workdir)
	assert.Error(t, err, "Should error on non-existent workdir")
	assert.Nil(t, sandbox, "Sandbox should be nil on error")
}

func TestSandboxWithEmptyWorkdir(t *testing.T) {
	// Test sandbox setup with empty workdir.
	workdir := ""

	sandbox, err := SetupSandbox(t, workdir)
	assert.Error(t, err, "Should error on empty workdir")
	assert.Nil(t, sandbox, "Sandbox should be nil on error")
	if err != nil {
		assert.Contains(t, err.Error(), "workdir cannot be empty", "Error should mention empty workdir")
	}
}

func TestSandboxMultipleComponentTypes(t *testing.T) {
	// Test sandbox with scenarios that have multiple component types.
	testCases := []struct {
		name     string
		workdir  string
		expected map[string]bool // expected environment variables
	}{
		{
			name:    "terraform only",
			workdir: "../fixtures/scenarios/env",
			expected: map[string]bool{
				"ATMOS_COMPONENTS_TERRAFORM_BASE_PATH": true,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sandbox, err := SetupSandbox(t, tc.workdir)
			if err != nil {
				// Some test scenarios might not exist, that's okay.
				t.Skipf("Skipping test: %v", err)
			}
			defer sandbox.Cleanup()

			envVars := sandbox.GetEnvironmentVariables()
			for key, shouldExist := range tc.expected {
				_, exists := envVars[key]
				if shouldExist {
					assert.True(t, exists, "Expected environment variable %s to be set", key)
				} else {
					assert.False(t, exists, "Expected environment variable %s not to be set", key)
				}
			}
		})
	}
}

func TestSandboxExcludesArtifacts(t *testing.T) {
	// Test that sandbox correctly excludes terraform and other artifacts.
	// First, create a test scenario with artifacts.
	tempWorkdir, err := os.MkdirTemp("", "sandbox-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempWorkdir)

	// Create a component structure with artifacts that should be excluded.
	componentDir := filepath.Join(tempWorkdir, "components", "terraform", "test-component")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	// Create files that should be copied.
	mainTfPath := filepath.Join(componentDir, "main.tf")
	require.NoError(t, os.WriteFile(mainTfPath, []byte("# main terraform file"), 0o644))

	variablesTfPath := filepath.Join(componentDir, "variables.tf")
	require.NoError(t, os.WriteFile(variablesTfPath, []byte("# variables file"), 0o644))

	// Create artifacts that should NOT be copied.
	lockfilePath := filepath.Join(componentDir, ".terraform.lock.hcl")
	require.NoError(t, os.WriteFile(lockfilePath, []byte("# lockfile"), 0o644))

	terraformDir := filepath.Join(componentDir, ".terraform")
	require.NoError(t, os.MkdirAll(terraformDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(terraformDir, "providers.json"), []byte("{}"), 0o644))

	tfvarsPath := filepath.Join(componentDir, "test.terraform.tfvars.json")
	require.NoError(t, os.WriteFile(tfvarsPath, []byte("{}"), 0o644))

	tfplanPath := filepath.Join(componentDir, "test.planfile")
	require.NoError(t, os.WriteFile(tfplanPath, []byte("plan"), 0o644))

	// Create atmos.yaml in the workdir.
	atmosYaml := filepath.Join(tempWorkdir, "atmos.yaml")
	atmosContent := `
components:
  terraform:
    base_path: "components/terraform"
`
	require.NoError(t, os.WriteFile(atmosYaml, []byte(atmosContent), 0o644))

	// Setup sandbox.
	sandbox, err := SetupSandbox(t, tempWorkdir)
	require.NoError(t, err)
	defer sandbox.Cleanup()

	// Check that good files were copied.
	sandboxComponentDir := filepath.Join(sandbox.ComponentsPath, "terraform", "test-component")
	assert.FileExists(t, filepath.Join(sandboxComponentDir, "main.tf"), "main.tf should be copied")
	assert.FileExists(t, filepath.Join(sandboxComponentDir, "variables.tf"), "variables.tf should be copied")

	// Check that artifacts were NOT copied.
	assert.NoFileExists(t, filepath.Join(sandboxComponentDir, ".terraform.lock.hcl"), ".terraform.lock.hcl should not be copied")
	assert.NoDirExists(t, filepath.Join(sandboxComponentDir, ".terraform"), ".terraform directory should not be copied")
	assert.NoFileExists(t, filepath.Join(sandboxComponentDir, "test.terraform.tfvars.json"), "tfvars.json should not be copied")
	assert.NoFileExists(t, filepath.Join(sandboxComponentDir, "test.planfile"), "planfile should not be copied")
}

func TestSandboxWithSymlinks(t *testing.T) {
	// Test sandbox handling of symlinks.
	tempWorkdir, err := os.MkdirTemp("", "sandbox-symlink-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempWorkdir)

	// Create a component.
	componentDir := filepath.Join(tempWorkdir, "components", "terraform", "test")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(componentDir, "main.tf"), []byte("# main"), 0o644))

	// Create a symlink to another component.
	targetDir := filepath.Join(tempWorkdir, "components", "terraform", "linked")
	require.NoError(t, os.MkdirAll(filepath.Dir(targetDir), 0o755))
	err = os.Symlink(componentDir, targetDir)
	if err != nil {
		t.Skipf("Skipping symlink test: %v", err)
	}

	// Create atmos.yaml.
	atmosYaml := filepath.Join(tempWorkdir, "atmos.yaml")
	atmosContent := `
components:
  terraform:
    base_path: "components/terraform"
`
	require.NoError(t, os.WriteFile(atmosYaml, []byte(atmosContent), 0o644))

	// Setup sandbox.
	sandbox, err := SetupSandbox(t, tempWorkdir)
	require.NoError(t, err)
	defer sandbox.Cleanup()

	// Both the original and linked component should exist.
	assert.FileExists(t, filepath.Join(sandbox.ComponentsPath, "terraform", "test", "main.tf"))
	// The symlink should be resolved and the content copied.
	linkedPath := filepath.Join(sandbox.ComponentsPath, "terraform", "linked")
	if info, err := os.Lstat(linkedPath); err == nil {
		// The linked directory should exist (either as a copy or a symlink).
		assert.True(t, info.IsDir() || info.Mode()&os.ModeSymlink != 0)
	}
}

func TestSandboxConcurrent(t *testing.T) {
	// Test that multiple sandboxes can be created concurrently.
	workdir := "../fixtures/scenarios/env"
	const numSandboxes = 5

	type result struct {
		sandbox *SandboxEnvironment
		err     error
	}

	results := make(chan result, numSandboxes)

	// Create sandboxes concurrently.
	for i := 0; i < numSandboxes; i++ {
		go func() {
			sandbox, err := SetupSandbox(t, workdir)
			results <- result{sandbox: sandbox, err: err}
		}()
	}

	// Collect results and verify.
	var sandboxes []*SandboxEnvironment
	for i := 0; i < numSandboxes; i++ {
		res := <-results
		require.NoError(t, res.err, "Sandbox %d creation should not error", i)
		require.NotNil(t, res.sandbox, "Sandbox %d should not be nil", i)
		sandboxes = append(sandboxes, res.sandbox)
	}

	// Verify all sandboxes have unique temp directories.
	tempDirs := make(map[string]bool)
	for _, sandbox := range sandboxes {
		assert.False(t, tempDirs[sandbox.TempDir], "Each sandbox should have a unique temp directory")
		tempDirs[sandbox.TempDir] = true
	}

	// Clean up all sandboxes.
	for _, sandbox := range sandboxes {
		sandbox.Cleanup()
	}

	// Verify all temp directories are cleaned up.
	for tempDir := range tempDirs {
		assert.NoDirExists(t, tempDir, "Temp directory should be cleaned up")
	}
}

func TestSandboxExtractComponentPaths(t *testing.T) {
	// Test the extractComponentPaths function.
	testCases := []struct {
		name      string
		atmosYAML string
		expected  map[string]string
	}{
		{
			name: "terraform only",
			atmosYAML: `
components:
  terraform:
    base_path: "components/terraform"
`,
			expected: map[string]string{
				"terraform": "components/terraform",
			},
		},
		{
			name: "terraform and helmfile",
			atmosYAML: `
components:
  terraform:
    base_path: "components/terraform"
  helmfile:
    base_path: "components/helmfile"
`,
			expected: map[string]string{
				"terraform": "components/terraform",
				"helmfile":  "components/helmfile",
			},
		},
		{
			name: "absolute paths",
			atmosYAML: `
components:
  terraform:
    base_path: "/absolute/path/terraform"
`,
			expected: map[string]string{
				"terraform": "/absolute/path/terraform",
			},
		},
		{
			name: "no components",
			atmosYAML: `settings: {}
`,
			expected: map[string]string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create temp directory with atmos.yaml.
			tempDir, err := os.MkdirTemp("", "extract-test-*")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir)

			atmosFile := filepath.Join(tempDir, "atmos.yaml")
			require.NoError(t, os.WriteFile(atmosFile, []byte(tc.atmosYAML), 0o644))

			// Extract component paths.
			paths, err := extractComponentPaths(tempDir)
			require.NoError(t, err)

			// Verify results.
			assert.Equal(t, len(tc.expected), len(paths), "Number of component paths should match")
			for key, expectedPath := range tc.expected {
				actualPath, exists := paths[key]
				assert.True(t, exists, "Component type %s should exist", key)
				assert.Equal(t, expectedPath, actualPath, "Path for %s should match", key)
			}
		})
	}
}

func TestSandboxWithInvalidAtmosYAML(t *testing.T) {
	// Test sandbox with invalid atmos.yaml - should fall back to defaults.
	tempDir, err := os.MkdirTemp("", "invalid-atmos-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create invalid atmos.yaml.
	atmosFile := filepath.Join(tempDir, "atmos.yaml")
	require.NoError(t, os.WriteFile(atmosFile, []byte("invalid: yaml: content:"), 0o644))

	// Setup should fall back to defaults gracefully.
	sandbox, err := SetupSandbox(t, tempDir)
	assert.NoError(t, err, "Should not error on invalid YAML (falls back to defaults)")
	assert.NotNil(t, sandbox, "Sandbox should not be nil")
	if sandbox != nil {
		defer sandbox.Cleanup()
		// Verify it created sandbox with default paths.
		assert.DirExists(t, sandbox.TempDir, "Should create temp directory")
	}
}

func TestSandboxCleanupIdempotency(t *testing.T) {
	// Test that calling Cleanup multiple times is safe.
	workdir := "../fixtures/scenarios/env"

	sandbox, err := SetupSandbox(t, workdir)
	require.NoError(t, err)

	// Call cleanup multiple times - should not panic.
	sandbox.Cleanup()
	sandbox.Cleanup()
	sandbox.Cleanup()

	// Temp directory should still be gone.
	assert.NoDirExists(t, sandbox.TempDir)
}

func TestSandboxWithLargeComponentTree(t *testing.T) {
	// Test sandbox performance with a large component tree.
	if testing.Short() {
		t.Skip("Skipping large component tree test in short mode")
	}

	tempWorkdir, err := os.MkdirTemp("", "sandbox-large-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempWorkdir)

	// Create many components.
	const numComponents = 50
	for i := 0; i < numComponents; i++ {
		componentDir := filepath.Join(tempWorkdir, "components", "terraform", fmt.Sprintf("component-%d", i))
		require.NoError(t, os.MkdirAll(componentDir, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(componentDir, "main.tf"),
			[]byte(fmt.Sprintf("# Component %d", i)),
			0o644,
		))
	}

	// Create atmos.yaml.
	atmosYaml := filepath.Join(tempWorkdir, "atmos.yaml")
	atmosContent := `
components:
  terraform:
    base_path: "components/terraform"
`
	require.NoError(t, os.WriteFile(atmosYaml, []byte(atmosContent), 0o644))

	// Setup sandbox and measure time.
	sandbox, err := SetupSandbox(t, tempWorkdir)
	require.NoError(t, err)
	defer sandbox.Cleanup()

	// Verify all components were copied.
	for i := 0; i < numComponents; i++ {
		componentPath := filepath.Join(sandbox.ComponentsPath, "terraform", fmt.Sprintf("component-%d", i), "main.tf")
		assert.FileExists(t, componentPath, "Component %d should be copied", i)
	}
}

func TestSandboxPermissionsPreserved(t *testing.T) {
	// Test that file permissions are preserved in sandbox.
	tempWorkdir, err := os.MkdirTemp("", "sandbox-perms-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempWorkdir)

	// Create a component with specific permissions.
	componentDir := filepath.Join(tempWorkdir, "components", "terraform", "test")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	// Create files with different permissions.
	executableScript := filepath.Join(componentDir, "script.sh")
	require.NoError(t, os.WriteFile(executableScript, []byte("#!/bin/bash\necho test"), 0o755))

	readonlyFile := filepath.Join(componentDir, "readonly.tf")
	require.NoError(t, os.WriteFile(readonlyFile, []byte("# readonly"), 0o444))

	// Create atmos.yaml.
	atmosYaml := filepath.Join(tempWorkdir, "atmos.yaml")
	atmosContent := `
components:
  terraform:
    base_path: "components/terraform"
`
	require.NoError(t, os.WriteFile(atmosYaml, []byte(atmosContent), 0o644))

	// Setup sandbox.
	sandbox, err := SetupSandbox(t, tempWorkdir)
	require.NoError(t, err)
	defer sandbox.Cleanup()

	// Check permissions are preserved.
	sandboxScript := filepath.Join(sandbox.ComponentsPath, "terraform", "test", "script.sh")
	if info, err := os.Stat(sandboxScript); err == nil {
		// Check if executable bit is set (at least for owner).
		assert.True(t, info.Mode()&0o100 != 0, "Script should remain executable")
	}

	sandboxReadonly := filepath.Join(sandbox.ComponentsPath, "terraform", "test", "readonly.tf")
	if info, err := os.Stat(sandboxReadonly); err == nil {
		// File should exist and be readable.
		assert.True(t, info.Mode()&0o400 != 0, "File should be readable")
	}
}

func TestSandboxWithNestedComponents(t *testing.T) {
	// Test sandbox with nested component structures.
	tempWorkdir, err := os.MkdirTemp("", "sandbox-nested-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempWorkdir)

	// Create nested component structure.
	nestedPaths := []string{
		"components/terraform/infrastructure/vpc/main.tf",
		"components/terraform/infrastructure/eks/cluster/main.tf",
		"components/terraform/applications/frontend/main.tf",
		"components/terraform/applications/backend/api/main.tf",
	}

	for _, path := range nestedPaths {
		fullPath := filepath.Join(tempWorkdir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
		require.NoError(t, os.WriteFile(fullPath, []byte(fmt.Sprintf("# %s", path)), 0o644))
	}

	// Create atmos.yaml.
	atmosYaml := filepath.Join(tempWorkdir, "atmos.yaml")
	atmosContent := `
components:
  terraform:
    base_path: "components/terraform"
`
	require.NoError(t, os.WriteFile(atmosYaml, []byte(atmosContent), 0o644))

	// Setup sandbox.
	sandbox, err := SetupSandbox(t, tempWorkdir)
	require.NoError(t, err)
	defer sandbox.Cleanup()

	// Verify all nested components were copied.
	for _, path := range nestedPaths {
		// Remove the "components/terraform/" prefix and check in sandbox.
		relativePath := strings.TrimPrefix(path, "components/terraform/")
		sandboxPath := filepath.Join(sandbox.ComponentsPath, "terraform", relativePath)
		assert.FileExists(t, sandboxPath, "Nested component %s should be copied", path)
	}
}

func TestSandboxEnvironmentIsolation(t *testing.T) {
	// Test that sandbox provides proper environment isolation.
	workdir := "../fixtures/scenarios/env"

	// Set an environment variable that might affect tests.
	originalValue := os.Getenv("ATMOS_COMPONENTS_TERRAFORM_BASE_PATH")
	os.Setenv("ATMOS_COMPONENTS_TERRAFORM_BASE_PATH", "/should/not/be/used")
	defer os.Setenv("ATMOS_COMPONENTS_TERRAFORM_BASE_PATH", originalValue)

	sandbox, err := SetupSandbox(t, workdir)
	require.NoError(t, err)
	defer sandbox.Cleanup()

	// The sandbox should override the environment variable.
	envVars := sandbox.GetEnvironmentVariables()
	if tfPath, exists := envVars["ATMOS_COMPONENTS_TERRAFORM_BASE_PATH"]; exists {
		assert.NotEqual(t, "/should/not/be/used", tfPath, "Sandbox should override existing env var")
		assert.Contains(t, tfPath, sandbox.TempDir, "Should use sandbox temp directory")
	}
}
