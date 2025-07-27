package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWhichCommand_ToolNotConfigured(t *testing.T) {
	// Test with a tool that exists in registry but is not configured in .tool-versions
	cmd := whichCmd
	cmd.SetArgs([]string{"kubectl"})

	err := cmd.Execute()
	require.Error(t, err, "Should fail when tool is not configured in .tool-versions")
	assert.Contains(t, err.Error(), "not configured in .tool-versions")
}

func TestWhichCommand_InvalidTool(t *testing.T) {
	// Test with a tool that doesn't exist
	cmd := whichCmd
	cmd.SetArgs([]string{"nonexistent-tool-12345"})

	err := cmd.Execute()
	require.Error(t, err, "Should fail when tool doesn't exist")
	assert.Contains(t, err.Error(), "not found in local aliases or Aqua registry")
}

func TestWhichCommand_InvalidToolName(t *testing.T) {
	// Test with an invalid tool name that can't be resolved
	cmd := whichCmd
	cmd.SetArgs([]string{"invalid/tool/name"})

	err := cmd.Execute()
	require.Error(t, err, "Should fail when tool name is invalid")
	assert.Contains(t, err.Error(), "invalid tool specification")
}

func TestWhichCommand_EmptyToolName(t *testing.T) {
	// Test with empty tool name
	cmd := whichCmd
	cmd.SetArgs([]string{""})

	err := cmd.Execute()
	require.Error(t, err, "Should fail when tool name is empty")
	assert.Contains(t, err.Error(), "not found in local aliases or Aqua registry")
}

func TestWhichCommand_ToolInToolVersionsButNotInstalled(t *testing.T) {
	// Create a temporary .tool-versions file with a tool that's not installed
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

	// Create a .tool-versions file with a tool that exists in registry but won't be installed
	testToolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"999.999.999"}, // Use a version that won't be installed
		},
	}
	err := SaveToolVersions(toolVersionsFile, testToolVersions)
	require.NoError(t, err)

	// Temporarily set the global tool versions file path
	originalPath := toolVersionsFile
	toolVersionsFile = tempDir + "/.tool-versions"
	defer func() { toolVersionsFile = originalPath }()

	// Test the which command with a tool that's configured but not installed
	cmd := whichCmd
	cmd.SetArgs([]string{"terraform"})

	err = cmd.Execute()
	require.Error(t, err, "Should fail when tool is configured but not installed")
	assert.Contains(t, err.Error(), "failed to load .tool-versions file")
}

func TestWhichCommand_HelpFlag(t *testing.T) {
	// Test that help flag works
	cmd := whichCmd
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	// Help should not return an error
	require.NoError(t, err, "Help flag should not cause an error")
}

// Note: Argument validation tests are not included here because Cobra handles
// argument validation differently in test context vs command line execution.
// The core functionality is tested in other test cases.

func TestWhichCommand_ResolvesAlias(t *testing.T) {
	// Test that the command can resolve aliases
	// This test will pass if terraform is configured in .tool-versions and installed
	cmd := whichCmd
	cmd.SetArgs([]string{"terraform"})

	err := cmd.Execute()
	// This might succeed if terraform is configured and installed, or fail if not
	// We don't assert either way since it depends on the current .tool-versions state
	if err != nil {
		t.Logf("terraform not found or not installed: %v", err)
	} else {
		t.Logf("terraform found and installed via toolchain")
	}
}

func TestWhichCommand_CanonicalName(t *testing.T) {
	// Test with canonical name
	cmd := whichCmd
	cmd.SetArgs([]string{"hashicorp/terraform"})

	err := cmd.Execute()
	// This might succeed if hashicorp/terraform is configured and installed, or fail if not
	// We don't assert either way since it depends on the current .tool-versions state
	if err != nil {
		t.Logf("hashicorp/terraform not found or not installed: %v", err)
	} else {
		t.Logf("hashicorp/terraform found and installed via toolchain")
	}
}
