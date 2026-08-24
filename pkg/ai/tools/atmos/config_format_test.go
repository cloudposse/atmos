package atmos

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

func TestNewConfigFormatTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewConfigFormatTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Equal(t, atmosConfig, tool.atmosConfig)
}

func TestConfigFormatTool_Name(t *testing.T) {
	tool := NewConfigFormatTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_config_format", tool.Name())
}

func TestConfigFormatTool_Description(t *testing.T) {
	tool := NewConfigFormatTool(&schema.AtmosConfiguration{})
	assert.Contains(t, tool.Description(), "Format the active atmos.yaml")
}

func TestConfigFormatTool_Parameters(t *testing.T) {
	tool := NewConfigFormatTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 1)
	assert.Equal(t, "file", params[0].Name)
	assert.False(t, params[0].Required)
}

func TestConfigFormatTool_RequiresPermission(t *testing.T) {
	tool := NewConfigFormatTool(&schema.AtmosConfiguration{})
	assert.True(t, tool.RequiresPermission())
}

func TestConfigFormatTool_IsRestricted(t *testing.T) {
	tool := NewConfigFormatTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestConfigFormatTool_Execute(t *testing.T) {
	tool := NewConfigFormatTool(&schema.AtmosConfiguration{})
	ctx := context.Background()

	t.Run("formats the file in place", func(t *testing.T) {
		dir := t.TempDir()
		file := writeConfigFixture(t, dir, "settings: {enabled: true}\n")

		result, err := tool.Execute(ctx, map[string]interface{}{
			"file": file,
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Contains(t, result.Output, "Formatted")

		got, getErr := atmosyaml.GetFile(file, "settings.enabled")
		require.NoError(t, getErr)
		assert.Equal(t, "true", got)

		formatted, readErr := os.ReadFile(file)
		require.NoError(t, readErr)
		assert.NotEmpty(t, formatted)
	})

	t.Run("fails when the explicit file override does not exist", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"file": filepath.Join(t.TempDir(), "does-not-exist.yaml"),
		})

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIConfigFileNotFound)
	})

	t.Run("fails with malformed yaml content", func(t *testing.T) {
		dir := t.TempDir()
		file := writeConfigFixture(t, dir, "settings: {enabled: true\n")

		result, err := tool.Execute(ctx, map[string]interface{}{
			"file": file,
		})

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, atmosyaml.ErrInvalidYAMLExpression)
	})
}
