package toolchain

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPathCommand_WithInstalledTools(t *testing.T) {
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

	kubectlPath := filepath.Join(toolsDir, "bin", "kubernetes", "kubectl", "1.28.0")
	err = os.MkdirAll(kubectlPath, 0o755)
	require.NoError(t, err)

	kubectlBinary := filepath.Join(kubectlPath, "kubectl")
	err = os.WriteFile(kubectlBinary, []byte("mock kubectl binary"), 0o755)
	require.NoError(t, err)

	// Test path command
	cmd := pathCmd
	cmd.SetArgs([]string{})
	// Set the global flags
	ToolChainCmd.PersistentFlags().Set("tool-versions", toolVersionsFile)
	ToolChainCmd.PersistentFlags().Set("tools-dir", toolsDir)
	err = cmd.Execute()
	require.NoError(t, err, "Should successfully generate PATH with installed tools")
}

func TestPathCommand_ExportFlag(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")
	toolsDir := filepath.Join(tempDir, ".tools")

	// Create a .tool-versions file with a tool
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	// Create mock installed binary
	terraformPath := filepath.Join(toolsDir, "bin", "hashicorp", "terraform", "1.11.4")
	err = os.MkdirAll(terraformPath, 0o755)
	require.NoError(t, err)

	terraformBinary := filepath.Join(terraformPath, "terraform")
	err = os.WriteFile(terraformBinary, []byte("mock terraform binary"), 0o755)
	require.NoError(t, err)

	// Test path command with export flag
	cmd := pathCmd
	cmd.SetArgs([]string{"--export"})
	// Set the global flags
	ToolChainCmd.PersistentFlags().Set("tool-versions", toolVersionsFile)
	ToolChainCmd.PersistentFlags().Set("tools-dir", toolsDir)
	err = cmd.Execute()
	require.NoError(t, err, "Should successfully generate export PATH command")
}

func TestPathCommand_JSONFlag(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")
	toolsDir := filepath.Join(tempDir, ".tools")

	// Create a .tool-versions file with a tool
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	// Create mock installed binary
	terraformPath := filepath.Join(toolsDir, "bin", "hashicorp", "terraform", "1.11.4")
	err = os.MkdirAll(terraformPath, 0o755)
	require.NoError(t, err)

	terraformBinary := filepath.Join(terraformPath, "terraform")
	err = os.WriteFile(terraformBinary, []byte("mock terraform binary"), 0o755)
	require.NoError(t, err)

	// Test path command with JSON flag
	cmd := pathCmd
	cmd.SetArgs([]string{"--json"})
	// Set the global flags
	ToolChainCmd.PersistentFlags().Set("tool-versions", toolVersionsFile)
	ToolChainCmd.PersistentFlags().Set("tools-dir", toolsDir)
	err = cmd.Execute()
	require.NoError(t, err, "Should successfully generate JSON output")
}

func TestPathCommand_RelativeFlag(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")
	toolsDir := filepath.Join(tempDir, ".tools")

	// Create a .tool-versions file with a tool
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	// Create mock installed binary
	terraformPath := filepath.Join(toolsDir, "bin", "hashicorp", "terraform", "1.11.4")
	err = os.MkdirAll(terraformPath, 0o755)
	require.NoError(t, err)

	terraformBinary := filepath.Join(terraformPath, "terraform")
	err = os.WriteFile(terraformBinary, []byte("mock terraform binary"), 0o755)
	require.NoError(t, err)

	// Test path command with relative flag
	cmd := pathCmd
	cmd.SetArgs([]string{"--relative"})
	// Set the global flags
	ToolChainCmd.PersistentFlags().Set("tool-versions", toolVersionsFile)
	ToolChainCmd.PersistentFlags().Set("tools-dir", toolsDir)
	err = cmd.Execute()
	require.NoError(t, err, "Should successfully generate PATH with relative paths")
}

func TestPathCommand_EmptyToolVersionsFile(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")

	// Create an empty .tool-versions file
	toolVersions := &ToolVersions{
		Tools: map[string][]string{},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	// Test path command with empty file
	cmd := pathCmd
	cmd.SetArgs([]string{})
	// Set the global flag
	ToolChainCmd.PersistentFlags().Set("tool-versions", toolVersionsFile)
	err = cmd.Execute()
	require.Error(t, err, "Should error when no tools are configured")
	assert.Contains(t, err.Error(), "no tools installed from .tool-versions file")
}

func TestPathCommand_NonExistentToolVersionsFile(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, "non-existent")

	// Test path command with non-existent file
	cmd := pathCmd
	cmd.SetArgs([]string{})
	// Set the global flag
	ToolChainCmd.PersistentFlags().Set("tool-versions", toolVersionsFile)
	err := cmd.Execute()
	require.Error(t, err, "Should error when tool-versions file doesn't exist")
	assert.Contains(t, err.Error(), "no tools configured in tool-versions file")
}

func TestPathCommand_ToolsNotInstalled(t *testing.T) {
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

	// Test path command with tools that aren't installed
	ToolChainCmd.PersistentFlags().Set("tool-versions", toolVersionsFile)
	cmd := pathCmd
	cmd.SetArgs([]string{})
	err = cmd.Execute()
	require.Error(t, err, "Should error when no tools are installed")
	assert.Contains(t, err.Error(), "no installed tools found from tool-versions file")
}

func TestPathCommand_MixedInstalledAndNotInstalled(t *testing.T) {
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

	// Test path command with mixed tools
	ToolChainCmd.PersistentFlags().Set("tool-versions", toolVersionsFile)
	ToolChainCmd.PersistentFlags().Set("tools-dir", toolsDir)
	cmd := pathCmd
	cmd.SetArgs([]string{})
	err = cmd.Execute()
	require.NoError(t, err, "Should include only installed tools in PATH")
}

func TestPathCommand_WithCanonicalNames(t *testing.T) {
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

	// Test path command with canonical names
	ToolChainCmd.PersistentFlags().Set("tool-versions", toolVersionsFile)
	ToolChainCmd.PersistentFlags().Set("tools-dir", toolsDir)
	cmd := pathCmd
	cmd.SetArgs([]string{})
	err = cmd.Execute()
	require.Error(t, err, "Should fail when binary is not found")
	assert.Contains(t, err.Error(), "no installed tools found from tool-versions file")
}

func TestPathCommand_WithMultipleVersions(t *testing.T) {
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

	// Test path command with multiple versions
	ToolChainCmd.PersistentFlags().Set("tool-versions", toolVersionsFile)
	ToolChainCmd.PersistentFlags().Set("tools-dir", toolsDir)
	cmd := pathCmd
	cmd.SetArgs([]string{})
	err = cmd.Execute()
	require.NoError(t, err, "Should handle multiple versions correctly")
}

func TestPathCommand_NoArgs(t *testing.T) {
	cmd := pathCmd
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	// This should fail when no .tool-versions file exists
	require.Error(t, err, "Should fail when no .tool-versions file exists")
	assert.Contains(t, err.Error(), "no tools configured in tool-versions file")
}

func TestPathCommand_WithArgs(t *testing.T) {
	cmd := pathCmd
	cmd.SetArgs([]string{"extra", "args"})
	err := cmd.Execute()
	// The path command should fail when no .tool-versions file exists
	require.Error(t, err, "Should fail when no .tool-versions file exists")
	assert.Contains(t, err.Error(), "no tools configured in tool-versions file")
}

func TestPathCommand_CombinedFlags(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")
	toolsDir := filepath.Join(tempDir, ".tools")

	// Create a .tool-versions file with a tool
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	// Create mock installed binary
	terraformPath := filepath.Join(toolsDir, "bin", "hashicorp", "terraform", "1.11.4")
	err = os.MkdirAll(terraformPath, 0o755)
	require.NoError(t, err)

	terraformBinary := filepath.Join(terraformPath, "terraform")
	err = os.WriteFile(terraformBinary, []byte("mock terraform binary"), 0o755)
	require.NoError(t, err)

	// Test path command with combined flags (JSON + relative)
	ToolChainCmd.PersistentFlags().Set("tool-versions", toolVersionsFile)
	ToolChainCmd.PersistentFlags().Set("tools-dir", toolsDir)
	cmd := pathCmd
	cmd.SetArgs([]string{"--json", "--relative"})
	err = cmd.Execute()
	require.NoError(t, err, "Should handle combined flags correctly")
}

func TestPathCommand_IncludesCurrentPATH(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")
	toolsDir := filepath.Join(tempDir, ".tools")

	// Create a .tool-versions file with a tool
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	// Create mock installed binary
	terraformPath := filepath.Join(toolsDir, "bin", "hashicorp", "terraform", "1.11.4")
	err = os.MkdirAll(terraformPath, 0o755)
	require.NoError(t, err)

	terraformBinary := filepath.Join(terraformPath, "terraform")
	err = os.WriteFile(terraformBinary, []byte("mock terraform binary"), 0o755)
	require.NoError(t, err)

	// Set a test PATH environment variable
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)
	os.Setenv("PATH", "/test/path:/another/path")

	// Test path command
	ToolChainCmd.PersistentFlags().Set("tool-versions", toolVersionsFile)
	ToolChainCmd.PersistentFlags().Set("tools-dir", toolsDir)
	cmd := pathCmd
	cmd.SetArgs([]string{})
	err = cmd.Execute()
	require.NoError(t, err, "Should include current PATH in output")
}

func TestPathCommand_SortedOutput(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, ".tool-versions")
	toolsDir := filepath.Join(tempDir, ".tools")

	// Create a .tool-versions file with tools in non-alphabetical order
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"kubectl":   {"1.28.0"},
			"terraform": {"1.11.4"},
			"helm":      {"3.12.0"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	// Create mock installed binaries
	tools := []string{"kubectl", "terraform", "helm"}
	for _, tool := range tools {
		var owner, repo string
		switch tool {
		case "terraform":
			owner, repo = "hashicorp", "terraform"
		case "kubectl":
			owner, repo = "kubernetes", "kubectl"
		case "helm":
			owner, repo = "helm", "helm"
		}

		toolPath := filepath.Join(toolsDir, "bin", owner, repo, "1.11.4")
		err = os.MkdirAll(toolPath, 0o755)
		require.NoError(t, err)

		binary := filepath.Join(toolPath, tool)
		err = os.WriteFile(binary, []byte("mock binary"), 0o755)
		require.NoError(t, err)
	}

	// Test path command
	ToolChainCmd.PersistentFlags().Set("tool-versions", toolVersionsFile)
	ToolChainCmd.PersistentFlags().Set("tools-dir", toolsDir)
	cmd := pathCmd
	cmd.SetArgs([]string{})
	err = cmd.Execute()
	require.NoError(t, err, "Should produce sorted output")
}
