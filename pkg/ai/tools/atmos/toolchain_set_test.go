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

func TestNewToolchainSetTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewToolchainSetTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Same(t, atmosConfig, tool.atmosConfig)
}

func TestToolchainSetTool_Name(t *testing.T) {
	tool := NewToolchainSetTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_toolchain_set", tool.Name())
}

func TestToolchainSetTool_Description(t *testing.T) {
	tool := NewToolchainSetTool(&schema.AtmosConfiguration{})
	assert.NotEmpty(t, tool.Description())
}

func TestToolchainSetTool_Parameters(t *testing.T) {
	tool := NewToolchainSetTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 2)
	assert.Equal(t, paramTool, params[0].Name)
	assert.True(t, params[0].Required)
	assert.Equal(t, paramVersion, params[1].Name)
	assert.True(t, params[1].Required)
}

func TestToolchainSetTool_RequiresPermission(t *testing.T) {
	tool := NewToolchainSetTool(&schema.AtmosConfiguration{})
	assert.True(t, tool.RequiresPermission())
}

func TestToolchainSetTool_IsRestricted(t *testing.T) {
	tool := NewToolchainSetTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestToolchainSetTool_Execute(t *testing.T) {
	toolVersionsFile := setupToolchainTestFile(t)
	tool := NewToolchainSetTool(&schema.AtmosConfiguration{})
	ctx := context.Background()

	t.Run("sets a tool's default version", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			paramTool:    "terraform",
			paramVersion: "1.11.4",
		})
		require.NoError(t, err)
		require.True(t, result.Success)

		toolVersions, err := toolchain.LoadToolVersions(toolVersionsFile)
		require.NoError(t, err)
		assert.Contains(t, toolVersions.Tools, "hashicorp/terraform")
		assert.Contains(t, toolVersions.Tools["hashicorp/terraform"], "1.11.4")
	})

	t.Run("fails with missing version", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			paramTool: "terraform",
		})
		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
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
