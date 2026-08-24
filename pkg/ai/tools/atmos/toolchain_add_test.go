package atmos

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

func TestNewToolchainAddTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewToolchainAddTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Same(t, atmosConfig, tool.atmosConfig)
}

func TestToolchainAddTool_Name(t *testing.T) {
	tool := NewToolchainAddTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_toolchain_add", tool.Name())
}

func TestToolchainAddTool_Description(t *testing.T) {
	tool := NewToolchainAddTool(&schema.AtmosConfiguration{})
	assert.NotEmpty(t, tool.Description())
}

func TestToolchainAddTool_Parameters(t *testing.T) {
	tool := NewToolchainAddTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 2)
	assert.Equal(t, paramTool, params[0].Name)
	assert.True(t, params[0].Required)
	assert.Equal(t, paramVersion, params[1].Name)
	assert.False(t, params[1].Required)
	assert.Equal(t, defaultToolVersion, params[1].Default)
}

func TestToolchainAddTool_RequiresPermission(t *testing.T) {
	tool := NewToolchainAddTool(&schema.AtmosConfiguration{})
	assert.True(t, tool.RequiresPermission())
}

func TestToolchainAddTool_IsRestricted(t *testing.T) {
	tool := NewToolchainAddTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func setupToolchainTestFile(t *testing.T) string {
	t.Helper()
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, toolchain.DefaultToolVersionsFilePath)
	toolchain.SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{VersionsFile: toolVersionsFile},
	})
	t.Cleanup(func() { toolchain.SetAtmosConfig(nil) })
	return toolVersionsFile
}

func TestToolchainAddTool_Execute(t *testing.T) {
	toolVersionsFile := setupToolchainTestFile(t)
	tool := NewToolchainAddTool(&schema.AtmosConfiguration{})
	ctx := context.Background()

	t.Run("adds a tool with an explicit version", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			paramTool:    "terraform",
			paramVersion: "1.11.4",
		})
		require.NoError(t, err)
		require.True(t, result.Success)
		assert.Contains(t, result.Output, "terraform")

		toolVersions, err := toolchain.LoadToolVersions(toolVersionsFile)
		require.NoError(t, err)
		assert.Contains(t, toolVersions.Tools["terraform"], "1.11.4")
	})

	t.Run("fails with missing tool", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			paramVersion: "1.11.4",
		})
		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})
}
