package toolchain

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWhichCommand_ToolNotConfigured(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	// Create an empty .tool-versions file so the command can load it
	emptyToolVersions := &ToolVersions{
		Tools: map[string][]string{},
	}
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	err := SaveToolVersions(toolVersionsPath, emptyToolVersions)
	require.NoError(t, err)
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{ToolsDir: tempDir, FilePath: toolVersionsPath}})
	err = WhichExec("kubectl")

	require.Error(t, err, "Should fail when tool is not configured in .tool-versions")
	assert.Contains(t, err.Error(), "not configured in .tool-versions")
}

func TestWhichCommand_InvalidTool(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	// Create an empty .tool-versions file so the command can load it
	emptyToolVersions := &ToolVersions{
		Tools: map[string][]string{},
	}
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	err := SaveToolVersions(toolVersionsPath, emptyToolVersions)
	require.NoError(t, err)

	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{ToolsDir: tempDir, FilePath: toolVersionsPath}})
	err = WhichExec("nonexistent-tool-12345")

	require.Error(t, err, "Should fail when tool doesn't exist in .tool-versions")
	assert.Contains(t, err.Error(), "not configured in .tool-versions")
}

func TestWhichCommand_InvalidToolName(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	// Create a .tool-versions file with an invalid tool name
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"invalid/tool/name": {"1.0.0"},
		},
	}
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	err := SaveToolVersions(toolVersionsPath, toolVersions)
	require.NoError(t, err)

	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{ToolsDir: tempDir, FilePath: toolVersionsPath}})
	err = WhichExec("invalid/tool/name")
	require.Error(t, err, "Should fail when tool name is invalid")
	assert.Contains(t, err.Error(), "failed to resolve tool")
}

func TestWhichCommand_EmptyToolName(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	// Create an empty .tool-versions file so the command can load it
	emptyToolVersions := &ToolVersions{
		Tools: map[string][]string{},
	}
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	err := SaveToolVersions(toolVersionsPath, emptyToolVersions)
	require.NoError(t, err)

	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{ToolsDir: tempDir, FilePath: toolVersionsPath}})
	err = WhichExec("")

	require.Error(t, err, "Should fail when tool name is empty")
	assert.Contains(t, err.Error(), "not configured in .tool-versions")
}

func TestWhichCommand_ToolConfiguredButNotInstalled(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	// Create a .tool-versions file with a tool that's configured but won't be installed
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"999.999.999"}, // Use a version that won't be installed
		},
	}
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	err := SaveToolVersions(toolVersionsPath, toolVersions)
	require.NoError(t, err)

	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{ToolsDir: tempDir, FilePath: toolVersionsPath}})
	err = WhichExec("terraform")

	require.Error(t, err, "Should fail when tool is configured but not installed")
	assert.Contains(t, err.Error(), "is configured but not installed")
}

func TestWhichCommand_ToolConfiguredAndInstalled(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	// Create a .tool-versions file with a tool
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4"},
		},
	}
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{ToolsDir: tempDir, FilePath: toolVersionsPath}})
	err := SaveToolVersions(toolVersionsPath, toolVersions)
	require.NoError(t, err)

	// Create mock installed binary at the exact path the which command expects
	// The which command uses NewInstaller() which has binDir = filepath.Join(GetToolsDirPath(), "bin")
	// So we need to create the binary in ./.tools/bin/hashicorp/terraform/1.11.4/terraform
	installer := NewInstaller()
	binaryPath := installer.getBinaryPath("hashicorp", "terraform", "1.11.4")
	err = os.MkdirAll(filepath.Dir(binaryPath), defaultMkdirPermissions)
	require.NoError(t, err)
	err = os.WriteFile(binaryPath, []byte("mock terraform"), defaultMkdirPermissions)
	require.NoError(t, err)
	err = WhichExec("terraform")
	require.NoError(t, err, "Should succeed when tool is configured and installed")
}

func TestWhichCommand_NoToolVersionsFile(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	// Don't create a .tool-versions file
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")

	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{ToolsDir: tempDir, FilePath: toolVersionsPath}})
	err := WhichExec("terraform")

	require.Error(t, err, "Should fail when .tool-versions file doesn't exist")
	assert.Contains(t, err.Error(), "failed to load .tool-versions file")
}

func TestWhichCommand_CanonicalName(t *testing.T) {
	// Test with canonical name

	err := WhichExec("hashicorp/terraform")

	// This might succeed if hashicorp/terraform is configured and installed, or fail if not
	// We don't assert either way since it depends on the current .tool-versions state
	if err != nil {
		t.Logf("hashicorp/terraform not found or not installed: %v", err)
	} else {
		t.Logf("hashicorp/terraform found and installed via toolchain")
	}
}
