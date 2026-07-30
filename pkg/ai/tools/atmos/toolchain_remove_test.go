package atmos

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

func TestNewToolchainRemoveTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewToolchainRemoveTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Same(t, atmosConfig, tool.atmosConfig)
}

func TestToolchainRemoveTool_Name(t *testing.T) {
	tool := NewToolchainRemoveTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_toolchain_remove", tool.Name())
}

func TestToolchainRemoveTool_Description(t *testing.T) {
	tool := NewToolchainRemoveTool(&schema.AtmosConfiguration{})
	assert.NotEmpty(t, tool.Description())
}

func TestToolchainRemoveTool_Parameters(t *testing.T) {
	tool := NewToolchainRemoveTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 2)
	assert.Equal(t, paramTool, params[0].Name)
	assert.True(t, params[0].Required)
	assert.Equal(t, paramVersion, params[1].Name)
	assert.False(t, params[1].Required)
}

func TestToolchainRemoveTool_RequiresPermission(t *testing.T) {
	tool := NewToolchainRemoveTool(&schema.AtmosConfiguration{})
	assert.True(t, tool.RequiresPermission())
}

func TestToolchainRemoveTool_IsRestricted(t *testing.T) {
	tool := NewToolchainRemoveTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestToolchainRemoveTool_Execute(t *testing.T) {
	toolVersionsFile := setupToolchainTestFile(t)
	require.NoError(t, toolchain.SaveToolVersions(toolVersionsFile, &toolchain.ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.9.8", "1.11.4"},
		},
	}))

	tool := NewToolchainRemoveTool(&schema.AtmosConfiguration{})
	ctx := context.Background()

	t.Run("removes a specific version", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			paramTool:    "terraform",
			paramVersion: "1.9.8",
		})
		require.NoError(t, err)
		require.True(t, result.Success)

		toolVersions, err := toolchain.LoadToolVersions(toolVersionsFile)
		require.NoError(t, err)
		assert.NotContains(t, toolVersions.Tools["terraform"], "1.9.8")
		assert.Contains(t, toolVersions.Tools["terraform"], "1.11.4")
	})

	t.Run("removes the whole tool when version omitted", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			paramTool: "terraform",
		})
		require.NoError(t, err)
		require.True(t, result.Success)

		toolVersions, err := toolchain.LoadToolVersions(toolVersionsFile)
		require.NoError(t, err)
		assert.NotContains(t, toolVersions.Tools, "terraform")
	})

	t.Run("fails for a tool that is not configured", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			paramTool: "nonexistent",
		})
		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, toolchain.ErrToolNotFound)
	})

	t.Run("fails with missing tool param", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{})
		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})
}
