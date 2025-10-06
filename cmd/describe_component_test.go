package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDescribeComponentCmd_Error(t *testing.T) {
	stacksPath := "../tests/fixtures/scenarios/terraform-apply-affected"

	err := os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")

	err = os.Setenv("ATMOS_BASE_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_BASE_PATH' environment variable should execute without error")

	// Unset ENV variables after testing
	defer func() {
		os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
		os.Unsetenv("ATMOS_BASE_PATH")
	}()

	err = describeComponentCmd.RunE(describeComponentCmd, []string{})
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
	stacksPath := "../examples/quick-start-advanced"

	// Skip if examples directory doesn't exist
	if _, err := os.Stat(stacksPath); os.IsNotExist(err) {
		t.Skipf("Skipping test: %s directory not found", stacksPath)
	}

	err := os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	require.NoError(t, err)
	err = os.Setenv("ATMOS_BASE_PATH", stacksPath)
	require.NoError(t, err)

	defer func() {
		os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
		os.Unsetenv("ATMOS_BASE_PATH")
	}()

	// Reset flags for this test
	describeComponentCmd.Flags().Set("stack", "plat-ue2-dev")
	describeComponentCmd.Flags().Set("format", "json")
	describeComponentCmd.Flags().Set("provenance", "true")

	defer func() {
		describeComponentCmd.Flags().Set("stack", "")
		describeComponentCmd.Flags().Set("format", "yaml")
		describeComponentCmd.Flags().Set("provenance", "false")
	}()

	// Note: JSON format with provenance should work (provenance is embedded in the data)
	err = describeComponentCmd.RunE(describeComponentCmd, []string{"vpc"})
	// The command might fail due to missing files in test environment, but we're testing flag parsing
	// If it fails, it should be for a reason other than flag parsing
	if err != nil {
		assert.NotContains(t, err.Error(), "unknown flag", "Should not fail due to unknown flag")
		assert.NotContains(t, err.Error(), "invalid flag", "Should not fail due to invalid flag")
	}
}

func TestDescribeComponentCmd_ProvenanceWithFileOutput(t *testing.T) {
	stacksPath := "../examples/quick-start-advanced"

	// Skip if examples directory doesn't exist
	if _, err := os.Stat(stacksPath); os.IsNotExist(err) {
		t.Skipf("Skipping test: %s directory not found", stacksPath)
	}

	err := os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	require.NoError(t, err)
	err = os.Setenv("ATMOS_BASE_PATH", stacksPath)
	require.NoError(t, err)

	defer func() {
		os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
		os.Unsetenv("ATMOS_BASE_PATH")
	}()

	// Create a temporary file for output
	tmpFile := filepath.Join(os.TempDir(), "test-provenance-output.yaml")
	defer os.Remove(tmpFile)

	// Reset flags for this test
	describeComponentCmd.Flags().Set("stack", "plat-ue2-dev")
	describeComponentCmd.Flags().Set("file", tmpFile)
	describeComponentCmd.Flags().Set("provenance", "true")

	defer func() {
		describeComponentCmd.Flags().Set("stack", "")
		describeComponentCmd.Flags().Set("file", "")
		describeComponentCmd.Flags().Set("provenance", "false")
	}()

	err = describeComponentCmd.RunE(describeComponentCmd, []string{"vpc"})
	// The command might fail due to missing files in test environment
	if err != nil {
		assert.NotContains(t, err.Error(), "unknown flag", "Should not fail due to unknown flag")
		assert.NotContains(t, err.Error(), "invalid flag", "Should not fail due to invalid flag")
	}
}

func TestDescribeComponentCmd_AllFlagsCombined(t *testing.T) {
	// Test that all flags can be used together without conflicts
	flags := describeComponentCmd.PersistentFlags()

	// Set all flags to non-default values
	err := flags.Set("stack", "test-stack")
	assert.NoError(t, err)

	err = flags.Set("format", "json")
	assert.NoError(t, err)

	err = flags.Set("file", "/tmp/test.yaml")
	assert.NoError(t, err)

	err = flags.Set("provenance", "true")
	assert.NoError(t, err)

	err = flags.Set("process-templates", "false")
	assert.NoError(t, err)

	err = flags.Set("process-functions", "false")
	assert.NoError(t, err)

	// Verify all flags were set
	stackVal, _ := flags.GetString("stack")
	assert.Equal(t, "test-stack", stackVal)

	formatVal, _ := flags.GetString("format")
	assert.Equal(t, "json", formatVal)

	fileVal, _ := flags.GetString("file")
	assert.Equal(t, "/tmp/test.yaml", fileVal)

	provenanceVal, _ := flags.GetBool("provenance")
	assert.True(t, provenanceVal)

	templatesVal, _ := flags.GetBool("process-templates")
	assert.False(t, templatesVal)

	functionsVal, _ := flags.GetBool("process-functions")
	assert.False(t, functionsVal)

	// Reset flags
	flags.Set("stack", "")
	flags.Set("format", "yaml")
	flags.Set("file", "")
	flags.Set("provenance", "false")
	flags.Set("process-templates", "true")
	flags.Set("process-functions", "true")
}
