package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveCommand_ValidTool(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

	// Create a .tool-versions file with some tools
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4", "1.9.8"},
			"kubectl":   {"1.28.0"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	// Test removing terraform
	cmd := removeCmd
	cmd.SetArgs([]string{"--file", toolVersionsFile, "terraform"})
	err = cmd.Execute()
	require.NoError(t, err, "Should successfully remove valid tool")

	// Verify terraform was removed but kubectl remains
	updatedToolVersions, err := LoadToolVersions(toolVersionsFile)
	require.NoError(t, err)
	assert.NotContains(t, updatedToolVersions.Tools, "terraform")
	assert.Contains(t, updatedToolVersions.Tools, "kubectl")
	assert.Contains(t, updatedToolVersions.Tools["kubectl"], "1.28.0")
}

func TestRemoveCommand_NonExistentTool(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

	// Create a .tool-versions file with some tools
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	// Test removing non-existent tool
	cmd := removeCmd
	cmd.SetArgs([]string{"--file", toolVersionsFile, "nonexistent"})
	err = cmd.Execute()
	require.Error(t, err, "Should error when removing non-existent tool")
	assert.Contains(t, err.Error(), "not found")

	// Verify file is unchanged
	updatedToolVersions, err := LoadToolVersions(toolVersionsFile)
	require.NoError(t, err)
	assert.Contains(t, updatedToolVersions.Tools, "terraform")
	assert.Contains(t, updatedToolVersions.Tools["terraform"], "1.11.4")
}

func TestRemoveCommand_EmptyFile(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

	// Create an empty .tool-versions file
	toolVersions := &ToolVersions{
		Tools: map[string][]string{},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	// Test removing from empty file
	cmd := removeCmd
	cmd.SetArgs([]string{"--file", toolVersionsFile, "terraform"})
	err = cmd.Execute()
	require.Error(t, err, "Should error when removing from empty file")
	assert.Contains(t, err.Error(), "not found")

	// Verify file remains empty
	updatedToolVersions, err := LoadToolVersions(toolVersionsFile)
	require.NoError(t, err)
	assert.Empty(t, updatedToolVersions.Tools)
}

func TestRemoveCommand_CustomFilePath(t *testing.T) {
	tempDir := t.TempDir()
	customFile := filepath.Join(tempDir, "custom-versions")

	// Create a custom file with some tools
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4"},
			"kubectl":   {"1.28.0"},
		},
	}
	err := SaveToolVersions(customFile, toolVersions)
	require.NoError(t, err)

	// Test removing with custom file path
	cmd := removeCmd
	cmd.SetArgs([]string{"--file", customFile, "terraform"})
	err = cmd.Execute()
	require.NoError(t, err, "Should successfully remove tool from custom file")

	// Verify terraform was removed from custom file
	updatedToolVersions, err := LoadToolVersions(customFile)
	require.NoError(t, err)
	assert.NotContains(t, updatedToolVersions.Tools, "terraform")
	assert.Contains(t, updatedToolVersions.Tools, "kubectl")
}

func TestRemoveCommand_CanonicalName(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

	// Create a .tool-versions file with canonical name
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"hashicorp/terraform": {"1.11.4"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	// Test removing with canonical name
	cmd := removeCmd
	cmd.SetArgs([]string{"--file", toolVersionsFile, "hashicorp/terraform"})
	err = cmd.Execute()
	require.NoError(t, err, "Should successfully remove tool with canonical name")

	// Verify canonical name was removed
	updatedToolVersions, err := LoadToolVersions(toolVersionsFile)
	require.NoError(t, err)
	assert.NotContains(t, updatedToolVersions.Tools, "hashicorp/terraform")
}

func TestRemoveCommand_MultipleVersions(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

	// Create a .tool-versions file with multiple versions
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4", "1.9.8", "1.8.0"},
			"kubectl":   {"1.28.0"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	// Test removing tool with multiple versions
	cmd := removeCmd
	cmd.SetArgs([]string{"--file", toolVersionsFile, "terraform"})
	err = cmd.Execute()
	require.NoError(t, err, "Should successfully remove tool with multiple versions")

	// Verify all versions were removed
	updatedToolVersions, err := LoadToolVersions(toolVersionsFile)
	require.NoError(t, err)
	assert.NotContains(t, updatedToolVersions.Tools, "terraform")
	assert.Contains(t, updatedToolVersions.Tools, "kubectl")
}

func TestRemoveCommand_NoArgs(t *testing.T) {
	cmd := removeCmd
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	require.Error(t, err, "Should fail with no arguments")
}

func TestRemoveCommand_TooManyArgs(t *testing.T) {
	cmd := removeCmd
	cmd.SetArgs([]string{"terraform", "extra"})
	err := cmd.Execute()
	require.Error(t, err, "Should fail with too many arguments")
}

func TestRemoveCommand_EmptyToolName(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

	// Create a .tool-versions file
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	// Test removing empty tool name
	cmd := removeCmd
	cmd.SetArgs([]string{"--file", toolVersionsFile, ""})
	err = cmd.Execute()
	require.Error(t, err, "Should error when removing empty tool name")
	assert.Contains(t, err.Error(), "empty tool argument")

	// Verify file is unchanged
	updatedToolVersions, err := LoadToolVersions(toolVersionsFile)
	require.NoError(t, err)
	assert.Contains(t, updatedToolVersions.Tools, "terraform")
}

func TestRemoveCommand_FileDoesNotExist(t *testing.T) {
	tempDir := t.TempDir()
	nonExistentFile := filepath.Join(tempDir, "non-existent")

	// Test removing from non-existent file
	cmd := removeCmd
	cmd.SetArgs([]string{"--file", nonExistentFile, "terraform"})
	err := cmd.Execute()
	require.Error(t, err, "Should fail when file does not exist")
	assert.Contains(t, err.Error(), "no such file or directory")
}

func TestRemoveCommand_InvalidFilePath(t *testing.T) {
	// Test removing with invalid file path
	cmd := removeCmd
	cmd.SetArgs([]string{"--file", "/invalid/path/.tool-versions", "terraform"})
	err := cmd.Execute()
	require.Error(t, err, "Should fail with invalid file path")
}

func TestRemoveCommand_PreservesOtherTools(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

	// Create a .tool-versions file with multiple tools
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4"},
			"kubectl":   {"1.28.0"},
			"helm":      {"3.12.0"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	// Test removing terraform
	cmd := removeCmd
	cmd.SetArgs([]string{"--file", toolVersionsFile, "terraform"})
	err = cmd.Execute()
	require.NoError(t, err, "Should successfully remove terraform")

	// Verify only terraform was removed, others remain
	updatedToolVersions, err := LoadToolVersions(toolVersionsFile)
	require.NoError(t, err)
	assert.NotContains(t, updatedToolVersions.Tools, "terraform")
	assert.Contains(t, updatedToolVersions.Tools, "kubectl")
	assert.Contains(t, updatedToolVersions.Tools, "helm")
	assert.Contains(t, updatedToolVersions.Tools["kubectl"], "1.28.0")
	assert.Contains(t, updatedToolVersions.Tools["helm"], "3.12.0")
}

func TestRemoveCommand_RemoveSpecificVersion(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4", "1.9.8", "1.8.0"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	cmd := removeCmd
	cmd.SetArgs([]string{"--file", toolVersionsFile, "terraform@1.9.8"})
	err = cmd.Execute()
	require.NoError(t, err, "Should remove specific version")

	updated, err := LoadToolVersions(toolVersionsFile)
	require.NoError(t, err)
	assert.Contains(t, updated.Tools, "terraform")
	assert.ElementsMatch(t, []string{"1.11.4", "1.8.0"}, updated.Tools["terraform"])
}

func TestRemoveCommand_RemoveLastVersion_RemovesTool(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	cmd := removeCmd
	cmd.SetArgs([]string{"--file", toolVersionsFile, "terraform@1.11.4"})
	err = cmd.Execute()
	require.NoError(t, err, "Should remove last version and tool entry")

	updated, err := LoadToolVersions(toolVersionsFile)
	require.NoError(t, err)
	assert.NotContains(t, updated.Tools, "terraform")
}

func TestRemoveCommand_RemoveNonExistentVersion(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4", "1.9.8"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	cmd := removeCmd
	cmd.SetArgs([]string{"--file", toolVersionsFile, "terraform@2.0.0"})
	err = cmd.Execute()
	require.Error(t, err, "Should error when removing non-existent version")
	assert.Contains(t, err.Error(), "not found")

	updated, err := LoadToolVersions(toolVersionsFile)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"1.11.4", "1.9.8"}, updated.Tools["terraform"])
}

func TestRemoveCommand_RemoveAllVersions(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4", "1.9.8"},
			"kubectl":   {"1.28.0"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	cmd := removeCmd
	cmd.SetArgs([]string{"--file", toolVersionsFile, "terraform"})
	err = cmd.Execute()
	require.NoError(t, err, "Should remove all versions of tool")

	updated, err := LoadToolVersions(toolVersionsFile)
	require.NoError(t, err)
	assert.NotContains(t, updated.Tools, "terraform")
	assert.Contains(t, updated.Tools, "kubectl")
}

func TestRemoveCommand_CanonicalNameWithVersion(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"hashicorp/terraform": {"1.11.4", "1.9.8"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	cmd := removeCmd
	cmd.SetArgs([]string{"--file", toolVersionsFile, "hashicorp/terraform@1.9.8"})
	err = cmd.Execute()
	require.NoError(t, err, "Should remove specific version for canonical name")

	updated, err := LoadToolVersions(toolVersionsFile)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"1.11.4"}, updated.Tools["hashicorp/terraform"])
}

func TestRemoveCommand_CanonicalNameAllVersions(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"hashicorp/terraform": {"1.11.4", "1.9.8"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	cmd := removeCmd
	cmd.SetArgs([]string{"--file", toolVersionsFile, "hashicorp/terraform"})
	err = cmd.Execute()
	require.NoError(t, err, "Should remove all versions for canonical name")

	updated, err := LoadToolVersions(toolVersionsFile)
	require.NoError(t, err)
	assert.NotContains(t, updated.Tools, "hashicorp/terraform")
}

func TestRemoveCommand_RemoveVersionFromToolWithOneVersion(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	cmd := removeCmd
	cmd.SetArgs([]string{"--file", toolVersionsFile, "terraform@1.11.4"})
	err = cmd.Execute()
	require.NoError(t, err, "Should remove the only version and tool entry")

	updated, err := LoadToolVersions(toolVersionsFile)
	require.NoError(t, err)
	assert.NotContains(t, updated.Tools, "terraform")
}

func TestRemoveCommand_RemoveNonExistentTool(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	cmd := removeCmd
	cmd.SetArgs([]string{"--file", toolVersionsFile, "nonexistent"})
	err = cmd.Execute()
	require.Error(t, err, "Should error when removing non-existent tool")

	updated, err := LoadToolVersions(toolVersionsFile)
	require.NoError(t, err)
	assert.Contains(t, updated.Tools, "terraform")
}

func TestRemoveCommand_RemoveNonExistentToolWithVersion(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	cmd := removeCmd
	cmd.SetArgs([]string{"--file", toolVersionsFile, "nonexistent@1.0.0"})
	err = cmd.Execute()
	require.Error(t, err, "Should error when removing non-existent tool with version")
	assert.Contains(t, err.Error(), "not found")

	updated, err := LoadToolVersions(toolVersionsFile)
	require.NoError(t, err)
	assert.Contains(t, updated.Tools, "terraform")
}
