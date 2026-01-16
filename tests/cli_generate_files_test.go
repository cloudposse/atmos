package tests

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tests/testhelpers"
)

// TestTerraformGenerateFiles tests the terraform generate files command.
func TestTerraformGenerateFiles(t *testing.T) {
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

	err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	require.NoError(t, err, "Unset 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")
	err = os.Unsetenv("ATMOS_BASE_PATH")
	require.NoError(t, err, "Unset 'ATMOS_BASE_PATH' environment variable should execute without error")

	t.Run("single component file generation", func(t *testing.T) {
		// Change to test fixture directory.
		t.Chdir("fixtures/scenarios/terraform-generate-files")

		// Clean up any previously generated files.
		cleanGeneratedFiles(t, "components/terraform/vpc")

		// Run generate files for a single component.
		cmd := atmosRunner.Command("terraform", "generate", "files", "vpc", "-s", "dev")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.NoError(t, err, "terraform generate files should succeed, stderr: %s", stderr.String())

		combinedOutput := stdout.String() + stderr.String()

		// Defer output logging.
		defer func() {
			if t.Failed() {
				t.Logf("\n=== Full output from failed test ===")
				t.Logf("Output (%d bytes):\n%s", len(combinedOutput), combinedOutput)
			}
		}()

		// Verify generated files exist.
		verifyGeneratedFilesExist(t, []string{
			"components/terraform/vpc/locals.tf",
			"components/terraform/vpc/metadata.json",
			"components/terraform/vpc/README.md",
		})

		// Clean up.
		cleanGeneratedFiles(t, "components/terraform/vpc")
	})

	t.Run("all components file generation", func(t *testing.T) {
		// Change to test fixture directory.
		t.Chdir("fixtures/scenarios/terraform-generate-files")

		// Clean up any previously generated files.
		cleanGeneratedFiles(t, "components/terraform/vpc")
		cleanGeneratedFiles(t, "components/terraform/s3-bucket")

		// Run generate files for all components.
		cmd := atmosRunner.Command("terraform", "generate", "files", "--all")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.NoError(t, err, "terraform generate files --all should succeed, stderr: %s", stderr.String())

		combinedOutput := stdout.String() + stderr.String()

		// Defer output logging.
		defer func() {
			if t.Failed() {
				t.Logf("\n=== Full output from failed test ===")
				t.Logf("Output (%d bytes):\n%s", len(combinedOutput), combinedOutput)
			}
		}()

		// Verify generated files exist for multiple components.
		verifyGeneratedFilesExist(t, []string{
			"components/terraform/vpc/locals.tf",
			"components/terraform/vpc/metadata.json",
		})

		// Clean up.
		cleanGeneratedFiles(t, "components/terraform/vpc")
		cleanGeneratedFiles(t, "components/terraform/s3-bucket")
	})

	t.Run("dry-run does not create files", func(t *testing.T) {
		// Change to test fixture directory.
		t.Chdir("fixtures/scenarios/terraform-generate-files")

		// Clean up any previously generated files.
		cleanGeneratedFiles(t, "components/terraform/vpc")

		// Run generate files with --dry-run.
		cmd := atmosRunner.Command("terraform", "generate", "files", "vpc", "-s", "dev", "--dry-run")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.NoError(t, err, "terraform generate files --dry-run should succeed, stderr: %s", stderr.String())

		combinedOutput := stdout.String() + stderr.String()

		// Defer output logging.
		defer func() {
			if t.Failed() {
				t.Logf("\n=== Full output from failed test ===")
				t.Logf("Output (%d bytes):\n%s", len(combinedOutput), combinedOutput)
			}
		}()

		// Verify files were NOT created (dry-run).
		verifyGeneratedFilesNotExist(t, []string{
			"components/terraform/vpc/locals.tf",
			"components/terraform/vpc/metadata.json",
			"components/terraform/vpc/README.md",
		})
	})

	t.Run("idempotent generation", func(t *testing.T) {
		// Change to test fixture directory.
		t.Chdir("fixtures/scenarios/terraform-generate-files")

		// Clean up any previously generated files.
		cleanGeneratedFiles(t, "components/terraform/vpc")

		// First run - should create files.
		cmd := atmosRunner.Command("terraform", "generate", "files", "vpc", "-s", "dev")
		var stdout1, stderr1 bytes.Buffer
		cmd.Stdout = &stdout1
		cmd.Stderr = &stderr1

		err := cmd.Run()
		require.NoError(t, err, "First generate should succeed, stderr: %s", stderr1.String())

		output1 := stdout1.String() + stderr1.String()
		assert.Contains(t, output1, "Created", "First run should show files as created")

		// Second run - should produce no output (all files unchanged).
		// When all files are unchanged, the generator produces no output.
		cmd2 := atmosRunner.Command("terraform", "generate", "files", "vpc", "-s", "dev")
		var stdout2, stderr2 bytes.Buffer
		cmd2.Stdout = &stdout2
		cmd2.Stderr = &stderr2

		err = cmd2.Run()
		require.NoError(t, err, "Second generate should succeed, stderr: %s", stderr2.String())

		output2 := strings.TrimSpace(stdout2.String() + stderr2.String())
		// When all files are unchanged, there's no output (idempotent behavior).
		// The absence of "Created" or "Updated" confirms idempotency.
		assert.NotContains(t, output2, "Created", "Second run should not show files as created")
		assert.NotContains(t, output2, "Updated", "Second run should not show files as updated")

		// Clean up.
		cleanGeneratedFiles(t, "components/terraform/vpc")
	})

	t.Run("clean removes generated files", func(t *testing.T) {
		// Change to test fixture directory.
		t.Chdir("fixtures/scenarios/terraform-generate-files")

		// Clean up any previously generated files.
		cleanGeneratedFiles(t, "components/terraform/vpc")

		// First generate files.
		cmd := atmosRunner.Command("terraform", "generate", "files", "vpc", "-s", "dev")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.NoError(t, err, "generate should succeed, stderr: %s", stderr.String())

		// Verify files exist.
		verifyGeneratedFilesExist(t, []string{
			"components/terraform/vpc/locals.tf",
		})

		// Run clean.
		cleanCmd := atmosRunner.Command("terraform", "generate", "files", "vpc", "-s", "dev", "--clean")
		var cleanStdout, cleanStderr bytes.Buffer
		cleanCmd.Stdout = &cleanStdout
		cleanCmd.Stderr = &cleanStderr

		err = cleanCmd.Run()
		require.NoError(t, err, "clean should succeed, stderr: %s", cleanStderr.String())

		// Verify files are removed.
		verifyGeneratedFilesNotExist(t, []string{
			"components/terraform/vpc/locals.tf",
			"components/terraform/vpc/metadata.json",
			"components/terraform/vpc/README.md",
		})
	})
}

// TestTerraformGenerateFilesInteractive tests interactive behavior.
func TestTerraformGenerateFilesInteractive(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
	}

	t.Run("fails gracefully in CI without component", func(t *testing.T) {
		// Change to test fixture directory.
		t.Chdir("fixtures/scenarios/terraform-generate-files")

		// Run without component in CI mode.
		cmd := atmosRunner.Command("terraform", "generate", "files")
		cmd.Env = append(os.Environ(), "CI=true")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()

		// Should fail in CI when no component is provided.
		require.Error(t, err, "Should fail in CI without component")

		combinedOutput := stdout.String() + stderr.String()

		// Defer output logging.
		defer func() {
			if t.Failed() {
				t.Logf("\n=== Full output from failed test ===")
				t.Logf("Output (%d bytes):\n%s", len(combinedOutput), combinedOutput)
			}
		}()

		// Should NOT show interactive selector in CI.
		assert.NotContains(t, combinedOutput, "Choose a component",
			"Should not show selector in CI environment")
	})

	t.Run("explicit component works in CI", func(t *testing.T) {
		// Change to test fixture directory.
		t.Chdir("fixtures/scenarios/terraform-generate-files")

		// Clean up any previously generated files.
		cleanGeneratedFiles(t, "components/terraform/vpc")

		// Run with explicit component in CI mode.
		cmd := atmosRunner.Command("terraform", "generate", "files", "vpc", "-s", "dev")
		cmd.Env = append(os.Environ(), "CI=true")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.NoError(t, err, "Should succeed with explicit component in CI, stderr: %s", stderr.String())

		combinedOutput := stdout.String() + stderr.String()

		// Defer output logging.
		defer func() {
			if t.Failed() {
				t.Logf("\n=== Full output from failed test ===")
				t.Logf("Output (%d bytes):\n%s", len(combinedOutput), combinedOutput)
			}
		}()

		// Should NOT show interactive selector.
		assert.NotContains(t, combinedOutput, "Choose a component",
			"Should not show selector with explicit component")

		// Clean up.
		cleanGeneratedFiles(t, "components/terraform/vpc")
	})
}

// cleanGeneratedFiles removes generated files from the specified component directory.
func cleanGeneratedFiles(t *testing.T, componentDir string) {
	t.Helper()
	filesToClean := []string{
		"locals.tf",
		"metadata.json",
		"README.md",
		"config.yaml",
		"bucket_config.tf",
		"global-context.json",
		"terraform-context.json",
		"override-context.json",
	}

	for _, file := range filesToClean {
		filePath := filepath.Join(componentDir, file)
		_ = os.Remove(filePath) // Ignore errors - file may not exist.
	}
}

// verifyGeneratedFilesExist checks that the specified files exist.
func verifyGeneratedFilesExist(t *testing.T, files []string) {
	t.Helper()
	for _, file := range files {
		fileAbs, err := filepath.Abs(file)
		require.NoError(t, err, "Failed to resolve absolute path for %q", file)

		_, err = os.Stat(fileAbs)
		assert.NoError(t, err, "Expected generated file to exist: %q", file)
	}
}

// verifyGeneratedFilesNotExist checks that the specified files do NOT exist.
func verifyGeneratedFilesNotExist(t *testing.T, files []string) {
	t.Helper()
	for _, file := range files {
		fileAbs, err := filepath.Abs(file)
		require.NoError(t, err, "Failed to resolve absolute path for %q", file)

		_, err = os.Stat(fileAbs)
		assert.True(t, os.IsNotExist(err), "Expected generated file to NOT exist: %q", file)
	}
}

// TestTerraformGenerateFilesHCLOutput tests HCL file generation.
func TestTerraformGenerateFilesHCLOutput(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
	}

	t.Run("generates valid HCL files", func(t *testing.T) {
		// Change to test fixture directory.
		t.Chdir("fixtures/scenarios/terraform-generate-files")

		// Clean up any previously generated files.
		cleanGeneratedFiles(t, "components/terraform/vpc")

		// Generate files.
		cmd := atmosRunner.Command("terraform", "generate", "files", "vpc", "-s", "dev")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.NoError(t, err, "generate should succeed, stderr: %s", stderr.String())

		// Read the generated HCL file.
		localsContent, err := os.ReadFile("components/terraform/vpc/locals.tf")
		require.NoError(t, err, "Should be able to read locals.tf")

		// Verify HCL content structure.
		content := string(localsContent)
		assert.Contains(t, content, "locals", "Should contain locals block")
		assert.Contains(t, content, "environment", "Should contain environment variable")
		assert.Contains(t, content, "vpc_name", "Should contain vpc_name variable")

		// Clean up.
		cleanGeneratedFiles(t, "components/terraform/vpc")
	})

	t.Run("generates valid JSON files", func(t *testing.T) {
		// Change to test fixture directory.
		t.Chdir("fixtures/scenarios/terraform-generate-files")

		// Clean up any previously generated files.
		cleanGeneratedFiles(t, "components/terraform/vpc")

		// Generate files.
		cmd := atmosRunner.Command("terraform", "generate", "files", "vpc", "-s", "dev")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.NoError(t, err, "generate should succeed, stderr: %s", stderr.String())

		// Read the generated JSON file.
		jsonContent, err := os.ReadFile("components/terraform/vpc/metadata.json")
		require.NoError(t, err, "Should be able to read metadata.json")

		// Verify JSON content structure.
		content := string(jsonContent)
		assert.True(t, strings.HasPrefix(strings.TrimSpace(content), "{"), "Should be valid JSON starting with {")
		assert.Contains(t, content, "component", "Should contain component field")
		assert.Contains(t, content, "stack", "Should contain stack field")

		// Clean up.
		cleanGeneratedFiles(t, "components/terraform/vpc")
	})
}
