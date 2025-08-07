package toolchain

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListCommand_WithInstalledTools(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")
	toolsDir := filepath.Join(tempDir, ".tools")

	// Create a .tool-versions file with some tools
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4"},
			"kubectl":   {"1.28.0"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	// Create mock installed binaries
	terraformPath := filepath.Join(toolsDir, "bin", "hashicorp", "terraform", "1.11.4")
	err = os.MkdirAll(terraformPath, 0o755)
	require.NoError(t, err)

	terraformBinary := filepath.Join(terraformPath, "terraform")
	err = os.WriteFile(terraformBinary, []byte("mock terraform binary"), 0o755)
	require.NoError(t, err)

	// Set modification time for testing
	modTime := time.Date(2023, 12, 1, 10, 30, 0, 0, time.UTC)
	err = os.Chtimes(terraformBinary, modTime, modTime)
	require.NoError(t, err)

	kubectlPath := filepath.Join(toolsDir, "bin", "kubernetes", "kubectl", "1.28.0")
	err = os.MkdirAll(kubectlPath, 0o755)
	require.NoError(t, err)

	kubectlBinary := filepath.Join(kubectlPath, "kubectl")
	err = os.WriteFile(kubectlBinary, []byte("mock kubectl binary"), 0o755)
	require.NoError(t, err)

	// Test listing tools
	cmd := listCmd
	cmd.SetArgs([]string{})
	// Set the global flags
	ToolChainCmd.PersistentFlags().Set("tool-versions", toolVersionsFile)
	ToolChainCmd.PersistentFlags().Set("tools-dir", toolsDir)
	err = cmd.Execute()
	require.NoError(t, err, "Should successfully list installed tools")
}

func TestListCommand_EmptyToolVersionsFile(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

	// Create an empty .tool-versions file
	toolVersions := &ToolVersions{
		Tools: map[string][]string{},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	// Test listing with empty file
	cmd := listCmd
	cmd.SetArgs([]string{})
	// Set the global flag
	ToolChainCmd.PersistentFlags().Set("tool-versions", toolVersionsFile)
	err = cmd.Execute()
	require.NoError(t, err, "Should handle empty tool-versions file gracefully")
}

func TestListCommand_NonExistentToolVersionsFile(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, "non-existent")

	// Test listing with non-existent file
	cmd := listCmd
	cmd.SetArgs([]string{})
	// Set the global flag
	ToolChainCmd.PersistentFlags().Set("tool-versions", toolVersionsFile)
	err := cmd.Execute()
	require.Error(t, err, "Should error when tool-versions file doesn't exist")
	assert.Contains(t, err.Error(), "no tool-versions file found")
}

func TestListCommand_ToolsNotInstalled(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

	// Create a .tool-versions file with tools that aren't installed
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4"},
			"kubectl":   {"1.28.0"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	// Test listing tools that aren't installed
	cmd := listCmd
	cmd.SetArgs([]string{})
	// Set the global flag
	ToolChainCmd.PersistentFlags().Set("tool-versions", toolVersionsFile)
	err = cmd.Execute()
	require.NoError(t, err, "Should handle tools that aren't installed gracefully")
}

func TestListCommand_MixedInstalledAndNotInstalled(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")
	toolsDir := filepath.Join(tempDir, ".tools")

	// Create a .tool-versions file with mixed tools
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4"}, // This will be installed
			"kubectl":   {"1.28.0"}, // This will not be installed
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	// Create only terraform binary
	terraformPath := filepath.Join(toolsDir, "bin", "hashicorp", "terraform", "1.11.4")
	err = os.MkdirAll(terraformPath, 0o755)
	require.NoError(t, err)

	terraformBinary := filepath.Join(terraformPath, "terraform")
	err = os.WriteFile(terraformBinary, []byte("mock terraform binary"), 0o755)
	require.NoError(t, err)

	// Test listing mixed tools
	cmd := listCmd
	cmd.SetArgs([]string{})
	// Set the global flags
	ToolChainCmd.PersistentFlags().Set("tool-versions", toolVersionsFile)
	ToolChainCmd.PersistentFlags().Set("tools-dir", toolsDir)
	err = cmd.Execute()
	require.NoError(t, err, "Should list only installed tools")
}

func TestListCommand_WithLatestVersion(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")
	toolsDir := filepath.Join(tempDir, ".tools")

	// Create a .tool-versions file with latest version
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"latest"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	// Create mock latest file
	latestPath := filepath.Join(toolsDir, "bin", "hashicorp", "terraform", "latest")
	err = os.MkdirAll(latestPath, 0o755)
	require.NoError(t, err)

	latestFile := filepath.Join(latestPath, "latest")
	err = os.WriteFile(latestFile, []byte("1.11.4"), 0o644)
	require.NoError(t, err)

	terraformBinary := filepath.Join(latestPath, "terraform")
	err = os.WriteFile(terraformBinary, []byte("mock terraform binary"), 0o755)
	require.NoError(t, err)

	// Test listing with latest version
	cmd := listCmd
	cmd.SetArgs([]string{})
	// Set the global flags
	ToolChainCmd.PersistentFlags().Set("tool-versions", toolVersionsFile)
	ToolChainCmd.PersistentFlags().Set("tools-dir", toolsDir)
	err = cmd.Execute()
	require.NoError(t, err, "Should handle latest version correctly")
}

func TestListCommand_NoArgs(t *testing.T) {
	cmd := listCmd
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	// This should fail when no .tool-versions file exists
	require.Error(t, err, "Should fail when no .tool-versions file exists")
	assert.Contains(t, err.Error(), "no tool-versions file found in current directory")
}

func TestListCommand_WithArgs(t *testing.T) {
	cmd := listCmd
	cmd.SetArgs([]string{"extra", "args"})
	err := cmd.Execute()
	// The list command should fail when no .tool-versions file exists
	require.Error(t, err, "Should fail when no .tool-versions file exists")
	assert.Contains(t, err.Error(), "no tool-versions file found in current directory")
}

func TestFormatFileSize(t *testing.T) {
	tests := []struct {
		size     int64
		expected string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1572864, "1.5 MB"},
		{1073741824, "1.0 GB"},
		{1610612736, "1.5 GB"},
	}

	for _, test := range tests {
		result := formatFileSize(test.size)
		assert.Equal(t, test.expected, result, "Size %d should format to %s", test.size, test.expected)
	}
}

func TestListCommand_WithCanonicalNames(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")
	toolsDir := filepath.Join(tempDir, ".tools")

	// Create a .tool-versions file with canonical names
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"hashicorp/terraform": {"1.11.4"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	// Create mock installed binary with canonical name
	terraformPath := filepath.Join(toolsDir, "bin", "hashicorp", "terraform", "1.11.4")
	err = os.MkdirAll(terraformPath, 0o755)
	require.NoError(t, err)

	terraformBinary := filepath.Join(terraformPath, "terraform")
	err = os.WriteFile(terraformBinary, []byte("mock terraform binary"), 0o755)
	require.NoError(t, err)

	// Test listing with canonical names
	cmd := listCmd
	cmd.SetArgs([]string{})
	// Set the global flags
	ToolChainCmd.PersistentFlags().Set("tool-versions", toolVersionsFile)
	ToolChainCmd.PersistentFlags().Set("tools-dir", toolsDir)
	err = cmd.Execute()
	require.NoError(t, err, "Should handle canonical names correctly")
}

func TestListCommand_WithMultipleVersions(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")
	toolsDir := filepath.Join(tempDir, ".tools")

	// Create a .tool-versions file with multiple versions
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4", "1.9.8"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	// Create mock installed binaries for both versions
	terraformPath1 := filepath.Join(toolsDir, "bin", "hashicorp", "terraform", "1.11.4")
	err = os.MkdirAll(terraformPath1, 0o755)
	require.NoError(t, err)

	terraformBinary1 := filepath.Join(terraformPath1, "terraform")
	err = os.WriteFile(terraformBinary1, []byte("mock terraform binary"), 0o755)
	require.NoError(t, err)

	terraformPath2 := filepath.Join(toolsDir, "bin", "hashicorp", "terraform", "1.9.8")
	err = os.MkdirAll(terraformPath2, 0o755)
	require.NoError(t, err)

	terraformBinary2 := filepath.Join(terraformPath2, "terraform")
	err = os.WriteFile(terraformBinary2, []byte("mock terraform binary"), 0o755)
	require.NoError(t, err)

	// Test listing with multiple versions
	cmd := listCmd
	cmd.SetArgs([]string{})
	// Set the global flags
	ToolChainCmd.PersistentFlags().Set("tool-versions", toolVersionsFile)
	ToolChainCmd.PersistentFlags().Set("tools-dir", toolsDir)
	err = cmd.Execute()
	require.NoError(t, err, "Should handle multiple versions correctly")
}
