package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDescribeComponentCmd_Error(t *testing.T) {
	tk := NewTestKit(t)

	// This test verifies that calling the command with no arguments returns an error.
	// The command requires exactly one argument (the component name).
	err := describeComponentCmd.RunE(describeComponentCmd, []string{})
	assert.Error(tk, err, "describe component command should return an error when called with no parameters")
}

func TestDescribeComponentCmd_ProvenanceFlag(t *testing.T) {
	// Test that the --provenance flag is properly registered
	// Use PersistentFlags() since that's where the flag is registered
	provenanceFlag := describeComponentCmd.PersistentFlags().Lookup("provenance")
	require.NotNil(t, provenanceFlag, "provenance flag should be registered")
	assert.Equal(t, "bool", provenanceFlag.Value.Type(), "provenance flag should be a boolean")
	assert.Equal(t, "false", provenanceFlag.DefValue, "provenance flag should default to false")
}

// TestDescribeComponentCmd_ProvenanceWithFormatJSON tests that provenance and format flags
// are correctly parsed and accepted. This is a flag parsing test, not a functional test.
func TestDescribeComponentCmd_ProvenanceWithFormatJSON(t *testing.T) {
	tk := NewTestKit(t)

	stacksPath := "examples/quick-start-advanced"

	// Skip if examples directory doesn't exist.
	if _, err := os.Stat(stacksPath); os.IsNotExist(err) {
		tk.Skipf("Skipping test: %s directory not found", stacksPath)
	}

	tk.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	tk.Setenv("ATMOS_BASE_PATH", stacksPath)

	// Set flags for this test.
	require.NoError(tk, describeComponentCmd.PersistentFlags().Set("stack", "plat-ue2-dev"))
	require.NoError(tk, describeComponentCmd.PersistentFlags().Set("format", "json"))
	require.NoError(tk, describeComponentCmd.PersistentFlags().Set("provenance", "true"))

	// Execute command - may fail due to missing files in test environment.
	// We're testing that flag parsing succeeds, not the full command execution.
	err := describeComponentCmd.RunE(describeComponentCmd, []string{"vpc"})
	if err != nil {
		// Verify the error is not due to flag parsing issues.
		errStr := err.Error()
		assert.NotContains(tk, errStr, "unknown flag", "Flag parsing should succeed")
		assert.NotContains(tk, errStr, "invalid flag", "Flag validation should succeed")
	}
}

// TestDescribeComponentCmd_ProvenanceWithFileOutput tests that provenance and file flags
// are correctly parsed and accepted. This is a flag parsing test, not a functional test.
func TestDescribeComponentCmd_ProvenanceWithFileOutput(t *testing.T) {
	tk := NewTestKit(t)

	stacksPath := "examples/quick-start-advanced"

	// Skip if examples directory doesn't exist.
	if _, err := os.Stat(stacksPath); os.IsNotExist(err) {
		tk.Skipf("Skipping test: %s directory not found", stacksPath)
	}

	tk.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	tk.Setenv("ATMOS_BASE_PATH", stacksPath)

	// Create a temporary file for output.
	tmpFile := filepath.Join(os.TempDir(), "test-provenance-output.yaml")
	defer os.Remove(tmpFile)

	// Set flags for this test.
	require.NoError(tk, describeComponentCmd.PersistentFlags().Set("stack", "plat-ue2-dev"))
	require.NoError(tk, describeComponentCmd.PersistentFlags().Set("file", tmpFile))
	require.NoError(tk, describeComponentCmd.PersistentFlags().Set("provenance", "true"))

	// Execute command - may fail due to missing files in test environment.
	// We're testing that flag parsing succeeds, not the full command execution.
	err := describeComponentCmd.RunE(describeComponentCmd, []string{"vpc"})
	if err != nil {
		// Verify the error is not due to flag parsing issues.
		errStr := err.Error()
		assert.NotContains(tk, errStr, "unknown flag", "Flag parsing should succeed")
		assert.NotContains(tk, errStr, "invalid flag", "Flag validation should succeed")
	}
}

// TestDescribeComponentCmd_PathResolution tests that component arguments with various formats
// are processed without panicking. This is a smoke test for the path resolution code path.
func TestDescribeComponentCmd_PathResolution(t *testing.T) {
	tk := NewTestKit(t)

	stacksPath := "examples/quick-start-advanced"

	// Skip if examples directory doesn't exist.
	if _, err := os.Stat(stacksPath); os.IsNotExist(err) {
		tk.Skipf("Skipping test: %s directory not found", stacksPath)
	}

	tk.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	tk.Setenv("ATMOS_BASE_PATH", stacksPath)

	tests := []struct {
		name      string
		component string
		stack     string
	}{
		{
			name:      "component name resolution",
			component: "vpc",
			stack:     "plat-ue2-dev",
		},
		{
			name:      "component name with slash",
			component: "vpc/security",
			stack:     "plat-ue2-dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tk := NewTestKit(t)

			// Set flags.
			require.NoError(tk, describeComponentCmd.PersistentFlags().Set("stack", tt.stack))

			// Execute command - may fail due to missing component in test environment.
			// We're testing that the code path executes without panicking.
			err := describeComponentCmd.RunE(describeComponentCmd, []string{tt.component})
			if err != nil {
				// Non-path components (without ./ or ../ prefix) should not trigger
				// path resolution logic, so any error should be about missing component.
				errStr := err.Error()
				assert.NotContains(tk, errStr, "path resolution", "Non-path component should bypass path resolution")
			}
		})
	}
}

// TestDescribeComponentCmd_ConfigLoadError tests that config load errors are properly handled
// for both regular component names and path-based component references.
func TestDescribeComponentCmd_ConfigLoadError(t *testing.T) {
	tests := []struct {
		name      string
		component string
	}{
		{
			name:      "non-path component with invalid config",
			component: "vpc",
		},
		{
			name:      "path component with invalid config",
			component: "./components/terraform/vpc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tk := NewTestKit(t)

			// Set invalid config path to trigger config load error.
			tk.Setenv("ATMOS_CLI_CONFIG_PATH", "/nonexistent/path")

			// Set flags.
			require.NoError(tk, describeComponentCmd.PersistentFlags().Set("stack", "test-stack"))

			// Run command - should fail due to config load error.
			err := describeComponentCmd.RunE(describeComponentCmd, []string{tt.component})
			assert.Error(tk, err, "Command should fail with invalid config path")
		})
	}
}

// TestDescribeComponentCmd_AuthManager tests that the auth manager code path is exercised
// without panicking. This is a smoke test for auth manager integration.
func TestDescribeComponentCmd_AuthManager(t *testing.T) {
	tk := NewTestKit(t)

	stacksPath := "examples/quick-start-advanced"

	// Skip if examples directory doesn't exist.
	if _, err := os.Stat(stacksPath); os.IsNotExist(err) {
		tk.Skipf("Skipping test: %s directory not found", stacksPath)
	}

	tk.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	tk.Setenv("ATMOS_BASE_PATH", stacksPath)

	// Set flags.
	require.NoError(tk, describeComponentCmd.PersistentFlags().Set("stack", "plat-ue2-dev"))

	// Execute command - may fail due to missing component in test environment.
	// We're testing that auth manager creation code path executes without panicking.
	// Actual auth validation is covered in dedicated auth tests.
	err := describeComponentCmd.RunE(describeComponentCmd, []string{"vpc"})
	if err != nil {
		// Verify error is not due to auth manager initialization issues.
		errStr := err.Error()
		assert.NotContains(tk, errStr, "auth manager creation failed", "Auth manager should initialize without errors")
	}
}
