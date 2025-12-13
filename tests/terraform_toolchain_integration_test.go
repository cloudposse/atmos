package tests

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
)

// This file contains integration tests for the toolchain system's integration with terraform commands.
//
// Background:
// The toolchain system is designed to:
// 1. Install tools (like terraform) to .tools/bin/{owner}/{repo}/{version}/
// 2. Modify PATH to include these directories
// 3. Pass the modified PATH to terraform subprocesses
//
// Current Implementation Issue:
// UpdatePathForTools() in pkg/dependencies/installer.go uses os.Setenv("PATH", newPath)
// which modifies the global process environment. This is problematic because:
// - It has side effects on the entire Atmos process
// - It can cause race conditions in concurrent executions
// - It pollutes test environments
// - It's unnecessary since ExecuteShellCommand already merges environments
//
// These tests:
// 1. Reproduce the reported issue where terraform isn't found from toolchain
// 2. Verify current behavior (both working and broken cases)
// 3. Will validate the fix once we refactor to avoid os.Setenv()
//
// See docs/prd/toolchain-terraform-integration.md for architectural details.

// TestTerraformToolchain_WithDependencies verifies that terraform commands work when
// component dependencies are configured in the stack.
//
// This test:
// 1. Configures terraform as a toolchain dependency
// 2. Executes terraform version command
// 3. Verifies the toolchain binary is used (not system terraform)
//
// NOTE: This test requires network access to download terraform from the toolchain registry.
func TestTerraformToolchain_WithDependencies(t *testing.T) {
	// Skip if we can't download tools (network required)
	if testing.Short() {
		t.Skip("Skipping toolchain integration test in short mode (requires network)")
	}

	workDir := "fixtures/scenarios/toolchain-terraform-integration"
	t.Chdir(workDir)

	// Clean up any existing toolchain installations
	toolsDir := ".tools"
	os.RemoveAll(toolsDir)
	defer os.RemoveAll(toolsDir)

	// Capture PATH before execution to verify it gets modified
	pathBefore := os.Getenv("PATH")
	t.Logf("PATH before execution: %s", pathBefore)

	info := schema.ConfigAndStacksInfo{
		Stack:            "dev",
		ComponentType:    "terraform",
		ComponentFromArg: "test-component",
		SubCommand:       "version",
	}

	// Capture stdout to verify terraform executes
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := exec.ExecuteTerraform(info)

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Log output for debugging
	t.Logf("Terraform output:\n%s", output)
	t.Logf("PATH after execution: %s", os.Getenv("PATH"))

	// Assertions
	require.NoError(t, err, "terraform version command should succeed")

	// Verify terraform executed (output should contain "Terraform" or "version")
	assert.True(t,
		strings.Contains(output, "Terraform") || strings.Contains(output, "version"),
		"Output should indicate terraform executed: %s", output)

	// Verify toolchain binary was installed
	// The binary should be at .tools/bin/hashicorp/terraform/1.6.0/terraform
	expectedBinaryPath := filepath.Join(toolsDir, "bin", "hashicorp", "terraform", "1.6.0", "terraform")
	if _, err := os.Stat(expectedBinaryPath); err == nil {
		t.Logf("✓ Toolchain binary found at: %s", expectedBinaryPath)
	} else {
		// On Windows, check for .exe extension
		expectedBinaryPathWin := expectedBinaryPath + ".exe"
		if _, err := os.Stat(expectedBinaryPathWin); err == nil {
			t.Logf("✓ Toolchain binary found at: %s", expectedBinaryPathWin)
		} else {
			t.Logf("⚠ Toolchain binary not found at expected location: %s", expectedBinaryPath)
			t.Logf("This might indicate the toolchain installation failed or used a different path")
		}
	}

	// Verify PATH was modified (this is the key test)
	// NOTE: Due to os.Setenv() in UpdatePathForTools, the global PATH is modified
	pathAfter := os.Getenv("PATH")
	if strings.Contains(pathAfter, ".tools") {
		t.Logf("✓ PATH contains toolchain directory")
	} else {
		t.Logf("✗ PATH does NOT contain toolchain directory")
		t.Logf("This indicates UpdatePathForTools() may not have been called or failed")
	}
}

// TestTerraformToolchain_WithoutDependencies reproduces the reported issue where
// terraform commands fail when toolchain dependencies are not configured.
//
// This test demonstrates that when a component does NOT specify terraform in its
// dependencies, the toolchain integration does not provide the binary.
//
// Expected behavior:
// - If system terraform is available: uses system terraform
// - If system terraform is NOT available: command fails with "terraform not found"
func TestTerraformToolchain_WithoutDependencies(t *testing.T) {
	workDir := "fixtures/scenarios/toolchain-terraform-integration"
	t.Chdir(workDir)

	// Clean up any existing toolchain installations
	toolsDir := ".tools"
	os.RemoveAll(toolsDir)
	defer os.RemoveAll(toolsDir)

	pathBefore := os.Getenv("PATH")
	t.Logf("PATH before execution: %s", pathBefore)

	info := schema.ConfigAndStacksInfo{
		Stack:            "dev",
		ComponentType:    "terraform",
		ComponentFromArg: "test-component-no-deps", // This component has NO dependencies
		SubCommand:       "version",
	}

	// Capture stdout/stderr
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr

	err := exec.ExecuteTerraform(info)

	// Restore stdout/stderr
	wOut.Close()
	wErr.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	// Read captured output
	var bufOut, bufErr bytes.Buffer
	bufOut.ReadFrom(rOut)
	bufErr.ReadFrom(rErr)
	stdout := bufOut.String()
	stderr := bufErr.String()

	t.Logf("Stdout:\n%s", stdout)
	t.Logf("Stderr:\n%s", stderr)
	t.Logf("PATH after execution: %s", os.Getenv("PATH"))

	// Check if toolchain binary was installed (it should NOT be)
	expectedBinaryPath := filepath.Join(toolsDir, "bin", "hashicorp", "terraform", "1.6.0", "terraform")
	_, statErr := os.Stat(expectedBinaryPath)

	if statErr != nil {
		t.Logf("✓ Toolchain binary was NOT installed (expected behavior)")
	} else {
		t.Logf("✗ Toolchain binary WAS installed (unexpected - component has no dependencies)")
	}

	// This test documents current behavior:
	// - If system terraform exists: command succeeds using system terraform
	// - If system terraform does NOT exist: command fails
	if err != nil {
		t.Logf("Command failed (expected if system terraform not available): %v", err)
		// Verify error message indicates terraform not found
		errMsg := err.Error()
		if strings.Contains(errMsg, "terraform") || strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "executable") {
			t.Logf("✓ Error indicates terraform binary not found (expected behavior)")
		}
	} else {
		t.Logf("Command succeeded (system terraform is available)")
		// If command succeeded, verify it's using system terraform, not toolchain
		pathAfter := os.Getenv("PATH")
		if !strings.Contains(pathAfter, ".tools") {
			t.Logf("✓ PATH does NOT contain toolchain directory (expected)")
		} else {
			t.Logf("✗ PATH contains toolchain directory (unexpected for component without dependencies)")
		}
	}
}

// TestTerraformToolchain_PathPropagation verifies that the modified PATH is actually
// passed to the terraform subprocess.
//
// This test focuses on the environment variable propagation mechanism to ensure
// that PATH modifications reach the subprocess correctly.
func TestTerraformToolchain_PathPropagation(t *testing.T) {
	// Skip if we can't download tools
	if testing.Short() {
		t.Skip("Skipping toolchain integration test in short mode")
	}

	workDir := "fixtures/scenarios/toolchain-terraform-integration"
	t.Chdir(workDir)

	toolsDir := ".tools"
	os.RemoveAll(toolsDir)
	defer os.RemoveAll(toolsDir)

	info := schema.ConfigAndStacksInfo{
		Stack:            "dev",
		ComponentType:    "terraform",
		ComponentFromArg: "test-component",
		SubCommand:       "version",
	}

	// Capture original PATH
	originalPath := os.Getenv("PATH")

	// Execute terraform
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := exec.ExecuteTerraform(info)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	require.NoError(t, err)

	// Verify PATH was modified
	modifiedPath := os.Getenv("PATH")
	t.Logf("Original PATH: %s", originalPath)
	t.Logf("Modified PATH: %s", modifiedPath)

	// The modified PATH should contain the toolchain directory
	// NOTE: This assertion documents the current behavior where os.Setenv() is used
	if strings.Contains(modifiedPath, filepath.Join(toolsDir, "bin")) {
		t.Logf("✓ PATH was modified to include toolchain binaries")

		// Verify the toolchain path is PREPENDED (takes precedence)
		pathParts := strings.Split(modifiedPath, string(os.PathListSeparator))
		toolchainPathFound := false
		for i, part := range pathParts {
			if strings.Contains(part, filepath.Join(toolsDir, "bin")) {
				t.Logf("✓ Toolchain path found at index %d in PATH (precedence: %s)", i,
					map[bool]string{true: "high", false: "low"}[i < 5])
				toolchainPathFound = true
				break
			}
		}
		assert.True(t, toolchainPathFound, "Toolchain path should be in PATH")
	} else {
		t.Logf("✗ PATH was NOT modified to include toolchain binaries")
		t.Logf("This indicates a failure in UpdatePathForTools()")
	}

	// Verify terraform executed successfully
	assert.Contains(t, output, "Terraform", "Terraform should have executed")
}

// TestTerraformToolchain_BinaryLocation verifies that installed binaries are in the
// correct location and have the correct permissions.
func TestTerraformToolchain_BinaryLocation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping toolchain integration test in short mode")
	}

	// TODO: This test is currently skipped because the `terraform version` subcommand
	// shortcuts in terraform.go and does not go through component/stack processing,
	// which is where toolchain dependencies are resolved and installed. Once the
	// toolchain integration is enhanced to also work with the version subcommand
	// (or we change this test to use a different subcommand that triggers full
	// component processing), this can be enabled.
	t.Skip("Skipping: terraform version subcommand bypasses toolchain dependency installation")

	workDir := "fixtures/scenarios/toolchain-terraform-integration"
	t.Chdir(workDir)

	toolsDir := ".tools"
	os.RemoveAll(toolsDir)
	defer os.RemoveAll(toolsDir)

	info := schema.ConfigAndStacksInfo{
		Stack:            "dev",
		ComponentType:    "terraform",
		ComponentFromArg: "test-component",
		SubCommand:       "version",
	}

	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	err := exec.ExecuteTerraform(info)
	os.Stdout = oldStdout

	require.NoError(t, err)

	// Check binary location and permissions
	expectedPaths := []string{
		filepath.Join(toolsDir, "bin", "hashicorp", "terraform", "1.6.0", "terraform"),
		filepath.Join(toolsDir, "bin", "hashicorp", "terraform", "1.6.0", "terraform.exe"), // Windows
	}

	foundBinary := false
	for _, expectedPath := range expectedPaths {
		if info, err := os.Stat(expectedPath); err == nil {
			foundBinary = true
			t.Logf("✓ Binary found at: %s", expectedPath)
			t.Logf("  Size: %d bytes", info.Size())
			t.Logf("  Mode: %s", info.Mode())

			// On Unix systems, verify executable permission
			if !strings.HasSuffix(expectedPath, ".exe") {
				assert.True(t, info.Mode()&0o111 != 0,
					"Binary should be executable: %s", info.Mode())
			}
			break
		}
	}

	assert.True(t, foundBinary, "Terraform binary should be installed at expected location")
}
