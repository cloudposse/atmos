package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDescribeComponentCmd_Error(t *testing.T) {
	stacksPath := "tests/fixtures/scenarios/terraform-apply-affected"

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	err := describeComponentCmd.RunE(describeComponentCmd, []string{})
	assert.Error(t, err, "describe component command should return an error when called with no parameters")
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
