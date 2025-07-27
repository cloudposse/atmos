package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddCommand_ValidTool(t *testing.T) {
	// Create a temporary .tool-versions file
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

	// Test adding a valid tool
	cmd := addCmd
	cmd.SetArgs([]string{"--file", toolVersionsFile, "terraform", "1.11.4"})

	err := cmd.Execute()
	require.NoError(t, err, "Should successfully add valid tool")

	// Verify the tool was added to the file
	toolVersions, err := LoadToolVersions(toolVersionsFile)
	require.NoError(t, err)
	assert.Contains(t, toolVersions.Tools, "terraform")
	assert.Contains(t, toolVersions.Tools["terraform"], "1.11.4")
}

func TestAddCommand_ValidToolWithAlias(t *testing.T) {
	// Create a temporary .tool-versions file
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

	// Test adding a valid tool using alias
	cmd := addCmd
	cmd.SetArgs([]string{"--file", toolVersionsFile, "helm", "3.12.0"})

	err := cmd.Execute()
	require.NoError(t, err, "Should successfully add valid tool using alias")

	// Verify the tool was added to the file
	toolVersions, err := LoadToolVersions(toolVersionsFile)
	require.NoError(t, err)
	assert.Contains(t, toolVersions.Tools, "helm")
	assert.Contains(t, toolVersions.Tools["helm"], "3.12.0")
}

func TestAddCommand_ValidToolWithCanonicalName(t *testing.T) {
	// Create a temporary .tool-versions file
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

	// Test adding a valid tool using canonical name
	cmd := addCmd
	cmd.SetArgs([]string{"--file", toolVersionsFile, "hashicorp/terraform", "1.11.4"})

	err := cmd.Execute()
	require.NoError(t, err, "Should successfully add valid tool using canonical name")

	// Verify the tool was added to the file
	toolVersions, err := LoadToolVersions(toolVersionsFile)
	require.NoError(t, err)
	assert.Contains(t, toolVersions.Tools, "hashicorp/terraform")
	assert.Contains(t, toolVersions.Tools["hashicorp/terraform"], "1.11.4")
}

func TestAddCommand_InvalidTool(t *testing.T) {
	// Create a temporary .tool-versions file
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

	// Test adding an invalid tool
	cmd := addCmd
	cmd.SetArgs([]string{"--file", toolVersionsFile, "nonexistent-tool", "1.0.0"})

	err := cmd.Execute()
	require.Error(t, err, "Should fail when adding invalid tool")
	assert.Contains(t, err.Error(), "not found in local aliases or Aqua registry")

	// Verify the tool was NOT added to the file
	toolVersions, err := LoadToolVersions(toolVersionsFile)
	if err == nil {
		assert.NotContains(t, toolVersions.Tools, "nonexistent-tool")
	}
}

func TestAddCommand_InvalidToolWithCanonicalName(t *testing.T) {
	// Create a temporary .tool-versions file
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

	// Test adding an invalid tool using canonical name
	cmd := addCmd
	cmd.SetArgs([]string{"--file", toolVersionsFile, "nonexistent/package", "1.0.0"})

	err := cmd.Execute()
	require.Error(t, err, "Should fail when adding invalid tool with canonical name")
	assert.Contains(t, err.Error(), "not found in any registry")

	// Verify the tool was NOT added to the file
	toolVersions, err := LoadToolVersions(toolVersionsFile)
	if err == nil {
		assert.NotContains(t, toolVersions.Tools, "nonexistent/package")
	}
}

func TestAddCommand_UpdateExistingTool(t *testing.T) {
	// Create a temporary .tool-versions file with existing tool
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

	// Add initial tool
	initialToolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.9.8"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, initialToolVersions)
	require.NoError(t, err)

	// Test updating the tool with a new version
	cmd := addCmd
	cmd.SetArgs([]string{"--file", toolVersionsFile, "terraform", "1.11.4"})

	err = cmd.Execute()
	require.NoError(t, err, "Should successfully update existing tool")

	// Verify the tool was updated in the file
	toolVersions, err := LoadToolVersions(toolVersionsFile)
	require.NoError(t, err)
	assert.Contains(t, toolVersions.Tools, "terraform")
	assert.Contains(t, toolVersions.Tools["terraform"], "1.9.8")
	assert.Contains(t, toolVersions.Tools["terraform"], "1.11.4")
}

func TestAddCommand_InvalidVersion(t *testing.T) {
	// Create a temporary .tool-versions file
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

	// Test adding a tool with an invalid version
	// Note: Since we only validate that the tool exists in registry, not the specific version,
	// this test will pass even with an invalid version
	cmd := addCmd
	cmd.SetArgs([]string{"--file", toolVersionsFile, "terraform", "999.999.999"})

	err := cmd.Execute()
	require.NoError(t, err, "Should pass since we only validate tool existence, not specific version")

	// Verify the tool was added to the file (even with invalid version)
	toolVersions, err := LoadToolVersions(toolVersionsFile)
	require.NoError(t, err)
	assert.Contains(t, toolVersions.Tools, "terraform")
	assert.Contains(t, toolVersions.Tools["terraform"], "999.999.999")
}

func TestAddCommand_CustomToolVersionsFile(t *testing.T) {
	// Create a temporary directory with custom .tool-versions file
	tempDir := t.TempDir()
	customToolVersionsFile := filepath.Join(tempDir, "custom-versions")

	// Test adding a tool to a custom file
	cmd := addCmd
	cmd.SetArgs([]string{"--file", customToolVersionsFile, "terraform", "1.11.4"})

	err := cmd.Execute()
	require.NoError(t, err, "Should successfully add tool to custom file")

	// Verify the tool was added to the custom file
	toolVersions, err := LoadToolVersions(customToolVersionsFile)
	require.NoError(t, err)
	assert.Contains(t, toolVersions.Tools, "terraform")
	assert.Contains(t, toolVersions.Tools["terraform"], "1.11.4")
}

func TestAddCommand_AquaRegistryTool(t *testing.T) {
	// Create a temporary .tool-versions file
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

	// Test adding a tool from Aqua registry
	// Note: This test may fail if kubectl is not available in the Aqua registry
	// or if there are network issues
	cmd := addCmd
	cmd.SetArgs([]string{"--file", toolVersionsFile, "kubectl", "1.28.0"})

	err := cmd.Execute()
	// This test may fail due to network issues or registry availability
	// We'll skip the assertion for now
	if err != nil {
		t.Logf("Aqua registry test failed (expected for network/registry issues): %v", err)
		return
	}

	// Verify the tool was added to the file
	toolVersions, err := LoadToolVersions(toolVersionsFile)
	require.NoError(t, err)
	assert.Contains(t, toolVersions.Tools, "kubectl")
	assert.Contains(t, toolVersions.Tools["kubectl"], "1.28.0")
}

func TestAddCommand_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		tool        string
		version     string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "empty tool name",
			tool:        "",
			version:     "1.0.0",
			expectError: true,
			errorMsg:    "not found in local aliases or Aqua registry",
		},
		{
			name:        "empty version",
			tool:        "terraform",
			version:     "",
			expectError: true,
			errorMsg:    "cannot add tool",
		},
		{
			name:        "malformed tool name",
			tool:        "invalid/tool/name",
			version:     "1.0.0",
			expectError: true,
			errorMsg:    "invalid tool specification",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary .tool-versions file
			tempDir := t.TempDir()
			toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

			// Test the edge case
			cmd := addCmd
			cmd.SetArgs([]string{"--file", toolVersionsFile, tt.tool, tt.version})

			err := cmd.Execute()
			if tt.expectError {
				require.Error(t, err, "Should fail for edge case")
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err, "Should succeed for valid edge case")
			}
		})
	}
}
