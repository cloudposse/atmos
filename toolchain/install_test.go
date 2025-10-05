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
	os.Setenv("HOME", dir)
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
	os.Setenv("HOME", tempDir)

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
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{FilePath: toolVersionsPath}})
	defer func() {
		SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{FilePath: originalToolVersionsFile}})
	}()

	// Test that runInstall with no arguments doesn't error
	// This prevents regression where the function might error when no specific tool is provided
	err = RunInstall("", false, false)
	assert.NoError(t, err)
}
