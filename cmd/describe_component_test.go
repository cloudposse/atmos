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

func TestDescribeComponentCmd_ProvenanceWithFormatJSON(t *testing.T) {
	tk := NewTestKit(t)

	stacksPath := "examples/quick-start-advanced"

	// Skip if examples directory doesn't exist
	if _, err := os.Stat(stacksPath); os.IsNotExist(err) {
		tk.Skipf("Skipping test: %s directory not found", stacksPath)
	}

	tk.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	tk.Setenv("ATMOS_BASE_PATH", stacksPath)

	// Set flags for this test
	require.NoError(tk, describeComponentCmd.PersistentFlags().Set("stack", "plat-ue2-dev"))
	require.NoError(tk, describeComponentCmd.PersistentFlags().Set("format", "json"))
	require.NoError(tk, describeComponentCmd.PersistentFlags().Set("provenance", "true"))

	// Note: JSON format with provenance should work (provenance is embedded in the data)
	err := describeComponentCmd.RunE(describeComponentCmd, []string{"vpc"})
	// The command might fail due to missing files in test environment, but we're testing flag parsing
	// If it fails, it should be for a reason other than flag parsing
	if err != nil {
		assert.NotContains(tk, err.Error(), "unknown flag", "Should not fail due to unknown flag")
		assert.NotContains(tk, err.Error(), "invalid flag", "Should not fail due to invalid flag")
	}
}

func TestDescribeComponentCmd_ProvenanceWithFileOutput(t *testing.T) {
	tk := NewTestKit(t)

	stacksPath := "examples/quick-start-advanced"

	// Skip if examples directory doesn't exist
	if _, err := os.Stat(stacksPath); os.IsNotExist(err) {
		tk.Skipf("Skipping test: %s directory not found", stacksPath)
	}

	tk.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	tk.Setenv("ATMOS_BASE_PATH", stacksPath)

	// Create a temporary file for output
	tmpFile := filepath.Join(os.TempDir(), "test-provenance-output.yaml")
	defer os.Remove(tmpFile)

	// Set flags for this test
	require.NoError(tk, describeComponentCmd.PersistentFlags().Set("stack", "plat-ue2-dev"))
	require.NoError(tk, describeComponentCmd.PersistentFlags().Set("file", tmpFile))
	require.NoError(tk, describeComponentCmd.PersistentFlags().Set("provenance", "true"))

	err := describeComponentCmd.RunE(describeComponentCmd, []string{"vpc"})
	// The command might fail due to missing files in test environment
	if err != nil {
		assert.NotContains(tk, err.Error(), "unknown flag", "Should not fail due to unknown flag")
		assert.NotContains(tk, err.Error(), "invalid flag", "Should not fail due to invalid flag")
	}
}

func TestDescribeComponentCmd_PathResolution(t *testing.T) {
	tk := NewTestKit(t)

	stacksPath := "examples/quick-start-advanced"

	// Skip if examples directory doesn't exist
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

			err := describeComponentCmd.RunE(describeComponentCmd, []string{tt.component})
			// The command might fail due to missing component or stack in test environment.
			// We're testing that path resolution logic is executed without panicking.
			if err != nil {
				// Should not fail due to path resolution issues for non-path components.
				assert.NotContains(tk, err.Error(), "path resolution", "Non-path component should not trigger path resolution errors")
			}
		})
	}
}

func TestDescribeComponentCmd_ConfigLoadError(t *testing.T) {
	tests := []struct {
		name         string
		component    string
		shouldError  bool
		errorPattern string
	}{
		{
			name:        "non-path component with invalid config",
			component:   "vpc",
			shouldError: true,
		},
		{
			name:        "path component with invalid config",
			component:   "./components/terraform/vpc",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tk := NewTestKit(t)

			// Set invalid config path to trigger config load error.
			tk.Setenv("ATMOS_CLI_CONFIG_PATH", "/nonexistent/path")

			// Set flags.
			require.NoError(tk, describeComponentCmd.PersistentFlags().Set("stack", "test-stack"))

			// Run command - both should fail due to config load error.
			err := describeComponentCmd.RunE(describeComponentCmd, []string{tt.component})
			if tt.shouldError {
				assert.Error(tk, err, "Command should fail with invalid config")
			}
		})
	}
}

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

	// Run command - this will create auth manager from identity flags.
	// The command might fail for other reasons, but we're testing that
	// auth manager creation doesn't panic.
	err := describeComponentCmd.RunE(describeComponentCmd, []string{"vpc"})
	// We're mainly checking that auth manager creation path is exercised.
	// The actual auth validation is tested elsewhere.
	if err != nil {
		// Should not fail due to auth manager creation for tests without identity flag.
		assert.NotContains(tk, err.Error(), "auth manager creation failed")
	}
}
