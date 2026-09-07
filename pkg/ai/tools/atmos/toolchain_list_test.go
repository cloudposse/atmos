package atmos

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

// setupToolchainListTestEnv points the toolchain package at an isolated .tool-versions file and
// install path for the duration of the test, restoring global toolchain state afterward (the
// toolchain package tracks its AtmosConfig in a package-level variable; see toolchain.SetAtmosConfig).
func setupToolchainListTestEnv(t *testing.T, versionsFileContent string) {
	t.Helper()

	tmpDir := t.TempDir()
	toolVersionsFile := filepath.Join(tmpDir, toolchain.DefaultToolVersionsFilePath)
	installPath := filepath.Join(tmpDir, ".tools")

	if versionsFileContent != "" {
		require.NoError(t, os.WriteFile(toolVersionsFile, []byte(versionsFileContent), filePermissions))
	}

	toolchain.SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: toolVersionsFile,
			InstallPath:  installPath,
		},
	})
	t.Cleanup(func() { toolchain.SetAtmosConfig(nil) })
}

func TestToolchainListTool_Interface(t *testing.T) {
	tool := NewToolchainListTool(&schema.AtmosConfiguration{})

	assert.Equal(t, "atmos_toolchain_list", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.False(t, tool.RequiresPermission())
	assert.False(t, tool.IsRestricted())

	params := tool.Parameters()
	require.Len(t, params, 1)
	assert.Equal(t, "tool", params[0].Name)
	assert.False(t, params[0].Required)
}

func TestToolchainListTool_NewToolchainListTool(t *testing.T) {
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	tool := NewToolchainListTool(config)

	assert.NotNil(t, tool)
	assert.Equal(t, config, tool.atmosConfig)
}

func TestToolchainListTool_Execute_NoToolVersionsFile(t *testing.T) {
	setupToolchainListTestEnv(t, "")

	tool := NewToolchainListTool(&schema.AtmosConfiguration{})
	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)

	entries, ok := result.Data["tools"].([]toolchainVersionEntry)
	require.True(t, ok)
	assert.Empty(t, entries)
}

func TestToolchainListTool_Execute_ListsConfiguredTools(t *testing.T) {
	setupToolchainListTestEnv(t, "hashicorp/terraform 1.6.0\n")

	tool := NewToolchainListTool(&schema.AtmosConfiguration{})
	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "hashicorp/terraform@1.6.0")
	assert.Contains(t, result.Output, "not installed")

	entries, ok := result.Data["tools"].([]toolchainVersionEntry)
	require.True(t, ok)
	require.Len(t, entries, 1)
	assert.Equal(t, "hashicorp/terraform", entries[0].Tool)
	assert.Equal(t, "1.6.0", entries[0].Version)
	assert.True(t, entries[0].Default)
	assert.False(t, entries[0].Installed)
}

func TestToolchainListTool_Execute_FiltersByToolName(t *testing.T) {
	setupToolchainListTestEnv(t, "hashicorp/terraform 1.6.0\nopentofu/opentofu 1.7.0\n")

	tool := NewToolchainListTool(&schema.AtmosConfiguration{})
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"tool": "opentofu/opentofu",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)

	entries, ok := result.Data["tools"].([]toolchainVersionEntry)
	require.True(t, ok)
	require.Len(t, entries, 1)
	assert.Equal(t, "opentofu/opentofu", entries[0].Tool)
}

func TestToolchainListTool_Execute_MultipleVersionsMarksDefault(t *testing.T) {
	setupToolchainListTestEnv(t, "hashicorp/terraform 1.6.0 1.5.0\n")

	tool := NewToolchainListTool(&schema.AtmosConfiguration{})
	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	require.NoError(t, err)
	entries, ok := result.Data["tools"].([]toolchainVersionEntry)
	require.True(t, ok)
	require.Len(t, entries, 2)
	assert.Equal(t, "1.6.0", entries[0].Version)
	assert.True(t, entries[0].Default)
	assert.Equal(t, "1.5.0", entries[1].Version)
	assert.False(t, entries[1].Default)
}
