package toolchain

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Add a mock ToolResolver for tests

func TestInstallResolvesAliasFromToolVersions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Chdir(dir)

	// Write a .tool-versions file with the full alias key
	toolVersionsPath := filepath.Join(dir, DefaultToolVersionsFilePath)
	err := os.WriteFile(toolVersionsPath, []byte("opentofu/opentofu 1.10.0\n"), defaultFileWritePermissions)
	require.NoError(t, err)

	// Write a minimal tools.yaml with the alias
	toolsYamlPath := filepath.Join(dir, "tools.yaml")

	toolsYaml := `aliases:
  opentofu: opentofu/opentofu
`
	err = os.WriteFile(toolsYamlPath, []byte(toolsYaml), defaultFileWritePermissions)
	require.NoError(t, err)

	// Simulate install: should resolve alias 'opentofu' to 'opentofu/opentofu'
	mockResolver := &mockToolResolver{
		mapping: map[string][2]string{
			"terraform": {"hashicorp", "terraform"},
			"opentofu":  {"opentofu", "opentofu"},
			"kubectl":   {"kubernetes", "kubectl"},
			"helm":      {"helm", "helm"},
			"helmfile":  {"helmfile", "helmfile"},
		},
	}
	prevConfig := atmosConfig
	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			FilePath: toolVersionsPath,
		},
	})
	t.Cleanup(func() { SetAtmosConfig(prevConfig) })
	installer := NewInstallerWithResolver(mockResolver)
	owner, repo, err := installer.parseToolSpec("opentofu")
	assert.NoError(t, err)
	assert.Equal(t, "opentofu", owner)
	assert.Equal(t, "opentofu", repo)

	// Now test the install logic (mock actual install)
	// This should not error, as the alias is resolved and found in .tool-versions
	// We'll just call the lookup logic directly
	toolVersions, err := LoadToolVersions(toolVersionsPath)
	assert.NoError(t, err)
	version, exists := toolVersions.Tools[owner+"/"+repo]
	assert.True(t, exists)
	assert.Equal(t, []string{"1.10.0"}, version)
}

func TestRunInstallWithNoArgs(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create a .tool-versions file with some tools
	toolVersionsPath := filepath.Join(tempDir, DefaultToolVersionsFilePath)
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4"},
			"helm":      {"3.17.4"},
		},
	}
	err := SaveToolVersions(toolVersionsPath, toolVersions)
	require.NoError(t, err)

	// Temporarily set the global toolVersionsFile variable
	originalToolVersionsFile := GetToolVersionsFilePath()
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: toolVersionsPath}})
	defer func() {
		SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: originalToolVersionsFile}})
	}()

	// Test that runInstall with no arguments doesn't error
	// This prevents regression where the function might error when no specific tool is provided
	err = RunInstall("", false, false)
	assert.NoError(t, err)
}

// TestRunInstall_WithValidToolSpec tests RunInstall with a valid tool@version specification.
func TestRunInstall_WithValidToolSpec(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create a .tool-versions file
	toolVersionsPath := filepath.Join(tempDir, DefaultToolVersionsFilePath)
	toolVersions := &ToolVersions{
		Tools: map[string][]string{},
	}
	err := SaveToolVersions(toolVersionsPath, toolVersions)
	require.NoError(t, err)

	// Set Atmos config
	originalToolVersionsFile := GetToolVersionsFilePath()
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: toolVersionsPath}})
	defer func() {
		SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: originalToolVersionsFile}})
	}()

	// Test installing a specific tool with version
	err = RunInstall("terraform@1.11.4", false, false)
	assert.NoError(t, err)

	// Verify the tool was added to .tool-versions (it uses DefaultToolVersionsFilePath which is .tool-versions in HOME)
	actualPath := DefaultToolVersionsFilePath
	updatedToolVersions, err := LoadToolVersions(actualPath)
	require.NoError(t, err)
	assert.Contains(t, updatedToolVersions.Tools, "terraform")
	assert.Contains(t, updatedToolVersions.Tools["terraform"], "1.11.4")
}

// TestRunInstall_WithSetAsDefault tests RunInstall with setAsDefault flag.
func TestRunInstall_WithSetAsDefault(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create a .tool-versions file with existing versions
	toolVersionsPath := filepath.Join(tempDir, DefaultToolVersionsFilePath)
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.3", "1.11.2"},
		},
	}
	err := SaveToolVersions(toolVersionsPath, toolVersions)
	require.NoError(t, err)

	// Set Atmos config
	originalToolVersionsFile := GetToolVersionsFilePath()
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: toolVersionsPath}})
	defer func() {
		SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: originalToolVersionsFile}})
	}()

	// Test installing with setAsDefault=true
	err = RunInstall("terraform@1.11.4", true, false)
	assert.NoError(t, err)

	// Verify the new version is first (default) in .tool-versions (uses DefaultToolVersionsFilePath)
	actualPath := DefaultToolVersionsFilePath
	updatedToolVersions, err := LoadToolVersions(actualPath)
	require.NoError(t, err)
	assert.Contains(t, updatedToolVersions.Tools, "terraform")
	assert.Equal(t, "1.11.4", updatedToolVersions.Tools["terraform"][0])
}

// TestRunInstall_WithInvalidToolSpec tests RunInstall with an invalid tool specification.
func TestRunInstall_WithInvalidToolSpec(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create a .tool-versions file
	toolVersionsPath := filepath.Join(tempDir, DefaultToolVersionsFilePath)
	toolVersions := &ToolVersions{
		Tools: map[string][]string{},
	}
	err := SaveToolVersions(toolVersionsPath, toolVersions)
	require.NoError(t, err)

	// Set Atmos config
	originalToolVersionsFile := GetToolVersionsFilePath()
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: toolVersionsPath}})
	defer func() {
		SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: originalToolVersionsFile}})
	}()

	// Test with invalid tool spec (no version)
	err = RunInstall("nonexistent-tool", false, false)
	assert.Error(t, err)
}

// TestRunInstall_WithCanonicalFormat tests RunInstall with owner/repo@version format.
func TestRunInstall_WithCanonicalFormat(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create a .tool-versions file
	toolVersionsPath := filepath.Join(tempDir, DefaultToolVersionsFilePath)
	toolVersions := &ToolVersions{
		Tools: map[string][]string{},
	}
	err := SaveToolVersions(toolVersionsPath, toolVersions)
	require.NoError(t, err)

	// Set Atmos config
	originalToolVersionsFile := GetToolVersionsFilePath()
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: toolVersionsPath}})
	defer func() {
		SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: originalToolVersionsFile}})
	}()

	// Test installing with canonical owner/repo@version format
	err = RunInstall("hashicorp/terraform@1.11.4", false, false)
	assert.NoError(t, err)

	// Verify the tool was added to .tool-versions (uses DefaultToolVersionsFilePath)
	// Note: The tool may be registered as "terraform" or "hashicorp/terraform" depending on alias resolution
	actualPath := DefaultToolVersionsFilePath
	updatedToolVersions, err := LoadToolVersions(actualPath)
	require.NoError(t, err)
	// Check for either key - the implementation may normalize to the shorter form
	terraformKey := ""
	if _, exists := updatedToolVersions.Tools["terraform"]; exists {
		terraformKey = "terraform"
	} else if _, exists := updatedToolVersions.Tools["hashicorp/terraform"]; exists {
		terraformKey = "hashicorp/terraform"
	}
	assert.NotEmpty(t, terraformKey, "Tool should be registered as either 'terraform' or 'hashicorp/terraform'")
	assert.Contains(t, updatedToolVersions.Tools[terraformKey], "1.11.4")
}

// TestRunInstall_WithLatestKeyword tests RunInstall with the "latest" version keyword.
func TestRunInstall_WithLatestKeyword(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create a .tool-versions file
	toolVersionsPath := filepath.Join(tempDir, DefaultToolVersionsFilePath)
	toolVersions := &ToolVersions{
		Tools: map[string][]string{},
	}
	err := SaveToolVersions(toolVersionsPath, toolVersions)
	require.NoError(t, err)

	// Set Atmos config
	originalToolVersionsFile := GetToolVersionsFilePath()
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: toolVersionsPath}})
	defer func() {
		SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: originalToolVersionsFile}})
	}()

	// Test installing with "latest" version
	// This should resolve to the actual latest version from the registry
	err = RunInstall("terraform@latest", false, false)
	assert.NoError(t, err)

	// Verify a version was added (we can't predict the exact version, but it should be there)
	actualPath := DefaultToolVersionsFilePath
	updatedToolVersions, err := LoadToolVersions(actualPath)
	require.NoError(t, err)
	assert.Contains(t, updatedToolVersions.Tools, "terraform")
	assert.NotEmpty(t, updatedToolVersions.Tools["terraform"])
}

// TestRunInstall_Reinstall tests RunInstall with reinstallFlag=true.
func TestRunInstall_Reinstall(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create a .tool-versions file with existing tools
	toolVersionsPath := filepath.Join(tempDir, DefaultToolVersionsFilePath)
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4"},
		},
	}
	err := SaveToolVersions(toolVersionsPath, toolVersions)
	require.NoError(t, err)

	// Set Atmos config
	originalToolVersionsFile := GetToolVersionsFilePath()
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: toolVersionsPath}})
	defer func() {
		SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: originalToolVersionsFile}})
	}()

	// Test reinstalling all tools from .tool-versions
	err = RunInstall("", false, true)
	assert.NoError(t, err)
}
