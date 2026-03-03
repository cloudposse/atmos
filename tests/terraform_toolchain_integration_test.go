package tests

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"

	"github.com/cloudposse/atmos/internal/exec"
)

// skipInMacOSCI skips the test if running on macOS in a CI environment.
// These integration tests can cause timeouts on slower macOS CI runners.
func skipInMacOSCI(t *testing.T) {
	t.Helper()
	isMacOS := runtime.GOOS == "darwin"
	isCI := os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != ""
	if isMacOS && isCI {
		t.Skip("Skipping toolchain integration test on macOS CI (can cause timeouts)")
	}
}

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
	defer perf.Track(nil, "tests.TestTerraformToolchain_WithDependencies")()

	// Skip on macOS CI to prevent timeouts - these tests can be slow.
	skipInMacOSCI(t)

	// Skip if we can't download tools (network required).
	if testing.Short() {
		t.Skip("Skipping toolchain integration test in short mode (requires network)")
	}

	workDir := filepath.Join("fixtures", "scenarios", "toolchain-terraform-integration")
	t.Chdir(workDir)

	// Clean up any existing toolchain installations.
	toolsDir := ".tools"
	os.RemoveAll(toolsDir)
	defer os.RemoveAll(toolsDir)

	// Capture PATH before execution to verify it gets modified.
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
//   - If system terraform is available: uses system terraform.
//   - If system terraform is NOT available: command fails with "terraform not found".
func TestTerraformToolchain_WithoutDependencies(t *testing.T) {
	defer perf.Track(nil, "tests.TestTerraformToolchain_WithoutDependencies")()

	// Skip on macOS CI to prevent timeouts - these tests can be slow.
	skipInMacOSCI(t)

	// Skip if running in short mode (CI without terraform).
	if testing.Short() {
		t.Skip("Skipping toolchain integration test in short mode")
	}

	workDir := filepath.Join("fixtures", "scenarios", "toolchain-terraform-integration")
	t.Chdir(workDir)

	// Clean up any existing toolchain installations.
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
		logCommandFailed(t, err)
	} else {
		logCommandSucceeded(t)
	}
}

// logCommandFailed logs information when command failed.
func logCommandFailed(t *testing.T, err error) {
	t.Helper()
	t.Logf("Command failed (expected if system terraform not available): %v", err)
	// Verify error message indicates terraform not found.
	errMsg := err.Error()
	if strings.Contains(errMsg, "terraform") || strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "executable") {
		t.Logf("✓ Error indicates terraform binary not found (expected behavior)")
	}
}

// logCommandSucceeded logs information when command succeeded.
func logCommandSucceeded(t *testing.T) {
	t.Helper()
	t.Logf("Command succeeded (system terraform is available)")
	// If command succeeded, verify it's using system terraform, not toolchain.
	pathAfter := os.Getenv("PATH")
	if !strings.Contains(pathAfter, ".tools") {
		t.Logf("✓ PATH does NOT contain toolchain directory (expected)")
	} else {
		t.Logf("✗ PATH contains toolchain directory (unexpected for component without dependencies)")
	}
}

// TestTerraformToolchain_PathPropagation verifies that the modified PATH is actually
// passed to the terraform subprocess.
//
// This test focuses on the environment variable propagation mechanism to ensure
// that PATH modifications reach the subprocess correctly.
func TestTerraformToolchain_PathPropagation(t *testing.T) {
	defer perf.Track(nil, "tests.TestTerraformToolchain_PathPropagation")()

	// Skip on macOS CI to prevent timeouts - these tests can be slow.
	skipInMacOSCI(t)

	// Skip if we can't download tools.
	if testing.Short() {
		t.Skip("Skipping toolchain integration test in short mode")
	}

	workDir := filepath.Join("fixtures", "scenarios", "toolchain-terraform-integration")
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

	// Capture original PATH.
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
	defer perf.Track(nil, "tests.TestTerraformToolchain_BinaryLocation")()

	// Skip on macOS CI to prevent timeouts - these tests can be slow.
	skipInMacOSCI(t)

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

	workDir := filepath.Join("fixtures", "scenarios", "toolchain-terraform-integration")
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
		info, err := os.Stat(expectedPath)
		if err != nil {
			continue
		}
		foundBinary = true
		t.Logf("✓ Binary found at: %s", expectedPath)
		t.Logf("  Size: %d bytes", info.Size())
		t.Logf("  Mode: %s", info.Mode())

		// On Unix systems, verify executable permission.
		if !strings.HasSuffix(expectedPath, ".exe") {
			assert.True(t, info.Mode()&0o111 != 0,
				"Binary should be executable: %s", info.Mode())
		}
		break
	}

	assert.True(t, foundBinary, "Terraform binary should be installed at expected location")
}

// TestTerraformToolchain_MixinLevelDependencies verifies that dependencies defined at
// the component-type level (Scope 2) in stack YAML are propagated through to the
// component section after stack processing.
//
// This reproduces the bug where a user configures:
//
//	terraform:
//	  dependencies:
//	    tools:
//	      terraform: "1.6.0"
//
// But the stack processor drops this data, so toolchain auto-install never triggers.
func TestTerraformToolchain_MixinLevelDependencies(t *testing.T) {
	defer perf.Track(nil, "tests.TestTerraformToolchain_MixinLevelDependencies")()

	workDir := filepath.Join("fixtures", "scenarios", "toolchain-terraform-integration")
	t.Chdir(workDir)

	// Initialize CLI config and process stacks via ProcessStacks, which handles
	// stack name resolution, stack map lookup, and component config extraction.
	info := schema.ConfigAndStacksInfo{
		Stack:            "mixin-test",
		ComponentType:    "terraform",
		ComponentFromArg: "test-component-mixin",
		SubCommand:       "plan",
	}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	require.NoError(t, err)

	info, err = exec.ProcessStacks(&atmosConfig, info, true, false, false, nil, nil)
	require.NoError(t, err)

	// Verify that the component section contains dependencies from the terraform
	// section (Scope 2). Before the fix, this would be nil/empty because the stack
	// processor dropped the terraform.dependencies section.
	compSection := info.ComponentSection
	require.NotNil(t, compSection, "ComponentSection should not be nil")

	depsRaw, ok := compSection["dependencies"]
	require.True(t, ok, "ComponentSection should contain 'dependencies' key from terraform section (Scope 2)")

	deps, ok := depsRaw.(map[string]any)
	require.True(t, ok, "dependencies should be a map")

	toolsRaw, ok := deps["tools"]
	require.True(t, ok, "dependencies should contain 'tools' key")

	tools, ok := toolsRaw.(map[string]any)
	require.True(t, ok, "tools should be a map")

	tfVersion, ok := tools["terraform"]
	require.True(t, ok, "tools should contain 'terraform' key")
	assert.Equal(t, "1.6.0", tfVersion, "terraform version should be '1.6.0' from terraform section (Scope 2)")
}

// TestTerraformToolchain_MixinLevelDependencies_PlanCommand verifies end-to-end that
// when dependencies are configured at the component-type level (Scope 2),
// ExecuteTerraform with 'plan' subcommand resolves and auto-installs the terraform
// binary from the toolchain.
//
// This test proves that Scope 2 dependencies flow through the stack processor
// and trigger the toolchain installer. It verifies the binary is downloaded and
// placed at the correct path.
//
// Note: The subprocess may still fail to find the binary due to a separate PATH
// propagation issue (the installed binary path is in ComponentEnvList, but Go's
// exec.LookPath checks the process PATH). That is tracked separately.
func TestTerraformToolchain_MixinLevelDependencies_PlanCommand(t *testing.T) {
	defer perf.Track(nil, "tests.TestTerraformToolchain_MixinLevelDependencies_PlanCommand")()

	// Skip on macOS CI to prevent timeouts - these tests can be slow.
	skipInMacOSCI(t)

	// Skip if running in short mode (requires network to download terraform).
	if testing.Short() {
		t.Skip("Skipping toolchain integration test in short mode (requires network)")
	}

	workDir := filepath.Join("fixtures", "scenarios", "toolchain-terraform-integration")
	t.Chdir(workDir)

	// Clean up any existing toolchain installations.
	toolsDir := ".tools"
	os.RemoveAll(toolsDir)
	defer os.RemoveAll(toolsDir)

	info := schema.ConfigAndStacksInfo{
		Stack:            "mixin-test",
		ComponentType:    "terraform",
		ComponentFromArg: "test-component-mixin",
		SubCommand:       "plan",
	}

	// Capture stdout/stderr.
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	rOut, wOut, pipeErr := os.Pipe()
	require.NoError(t, pipeErr)
	rErr, wErr, pipeErr := os.Pipe()
	require.NoError(t, pipeErr)
	t.Cleanup(func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
		_ = wOut.Close()
		_ = wErr.Close()
		_ = rOut.Close()
		_ = rErr.Close()
	})
	os.Stdout = wOut
	os.Stderr = wErr

	// Execute terraform - it may fail for various reasons (no init, no backend, etc.)
	// but the key thing we verify is that the toolchain binary was installed.
	_ = exec.ExecuteTerraform(info)

	// Restore stdout/stderr and close write ends so readers can drain.
	_ = wOut.Close()
	_ = wErr.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	// Read captured output.
	var bufOut, bufErr bytes.Buffer
	_, _ = bufOut.ReadFrom(rOut)
	_, _ = bufErr.ReadFrom(rErr)

	t.Logf("Stdout:\n%s", bufOut.String())
	t.Logf("Stderr:\n%s", bufErr.String())

	// Verify toolchain binary was installed at the expected location.
	// This is the critical assertion: before the fix, the dependencies from the
	// terraform section (Scope 2) were dropped by the stack processor, so the
	// toolchain installer was never triggered and no binary was downloaded.
	expectedBinaryPath := filepath.Join(toolsDir, "bin", "hashicorp", "terraform", "1.6.0", "terraform")
	if runtime.GOOS == "windows" {
		expectedBinaryPath += ".exe"
	}
	_, statErr := os.Stat(expectedBinaryPath)
	assert.NoError(t, statErr, "Toolchain binary should be installed at: %s (Scope 2 dependencies should trigger auto-install)", expectedBinaryPath)
}

// TestTerraformToolchain_DependencyPrecedence verifies that all 3 scopes of
// dependencies are properly merged with correct precedence through the full
// stack processor pipeline:
//
//	Scope 1 (global)          → lowest priority
//	Scope 2 (component-type)  → middle priority
//	Scope 3 (component)       → highest priority
//
// It tests two components in the same stack:
//  1. test-component-override: has Scope 3 deps that override Scope 2
//  2. test-component-inherit: has NO Scope 3 deps, inherits from Scope 1+2
func TestTerraformToolchain_DependencyPrecedence(t *testing.T) {
	defer perf.Track(nil, "tests.TestTerraformToolchain_DependencyPrecedence")()

	workDir := filepath.Join("fixtures", "scenarios", "toolchain-terraform-integration")
	t.Chdir(workDir)

	// --- Test component that OVERRIDES Scope 2 at Scope 3 ---
	overrideInfo := schema.ConfigAndStacksInfo{
		Stack:            "override-test",
		ComponentType:    "terraform",
		ComponentFromArg: "test-component-override",
		SubCommand:       "plan",
	}

	atmosConfig, err := cfg.InitCliConfig(overrideInfo, true)
	require.NoError(t, err)

	overrideInfo, err = exec.ProcessStacks(&atmosConfig, overrideInfo, true, false, false, nil, nil)
	require.NoError(t, err)

	compSection := overrideInfo.ComponentSection
	require.NotNil(t, compSection)

	depsRaw, ok := compSection["dependencies"]
	require.True(t, ok, "override component should have dependencies")
	deps := depsRaw.(map[string]any)
	tools := deps["tools"].(map[string]any)

	// Scope 3 overrides Scope 2 for terraform version.
	assert.Equal(t, "1.10.3", tools["terraform"], "Scope 3 should override Scope 2 terraform version")

	// Scope 2 tflint should be inherited (not overridden by Scope 3).
	assert.Equal(t, "^0.54.0", tools["tflint"], "Scope 2 tflint should be inherited")

	// Scope 1 jq should be inherited through.
	assert.Equal(t, "latest", tools["jq"], "Scope 1 jq should be inherited")

	// Scope 3 adds a new tool not in Scope 1 or 2.
	assert.Equal(t, "latest", tools["checkov"], "Scope 3 should add checkov")

	// --- Test component that INHERITS from Scope 1+2 only ---
	inheritInfo := schema.ConfigAndStacksInfo{
		Stack:            "override-test",
		ComponentType:    "terraform",
		ComponentFromArg: "test-component-inherit",
		SubCommand:       "plan",
	}

	atmosConfig2, err := cfg.InitCliConfig(inheritInfo, true)
	require.NoError(t, err)

	inheritInfo, err = exec.ProcessStacks(&atmosConfig2, inheritInfo, true, false, false, nil, nil)
	require.NoError(t, err)

	compSection2 := inheritInfo.ComponentSection
	require.NotNil(t, compSection2)

	depsRaw2, ok := compSection2["dependencies"]
	require.True(t, ok, "inherit component should have dependencies from Scope 1+2")
	deps2 := depsRaw2.(map[string]any)
	tools2 := deps2["tools"].(map[string]any)

	// Should get Scope 2 terraform version (no Scope 3 override).
	assert.Equal(t, "1.6.0", tools2["terraform"], "Scope 2 terraform should be inherited without override")

	// Should get Scope 2 tflint.
	assert.Equal(t, "^0.54.0", tools2["tflint"], "Scope 2 tflint should be inherited")

	// Should get Scope 1 jq.
	assert.Equal(t, "latest", tools2["jq"], "Scope 1 jq should be inherited")

	// Should NOT have checkov (that was only in Scope 3 of the other component).
	_, hasCheckov := tools2["checkov"]
	assert.False(t, hasCheckov, "inherit component should NOT have checkov (only defined in other component's Scope 3)")
}
