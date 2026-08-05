package atmos

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

func TestNewConfigDeleteTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewConfigDeleteTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Equal(t, atmosConfig, tool.atmosConfig)
}

func TestConfigDeleteTool_Name(t *testing.T) {
	tool := NewConfigDeleteTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_config_delete", tool.Name())
}

func TestConfigDeleteTool_Description(t *testing.T) {
	tool := NewConfigDeleteTool(&schema.AtmosConfiguration{})
	assert.Contains(t, tool.Description(), "Delete a value")
}

func TestConfigDeleteTool_Parameters(t *testing.T) {
	tool := NewConfigDeleteTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 2)
	assert.Equal(t, "path", params[0].Name)
	assert.True(t, params[0].Required)
	assert.Equal(t, "file", params[1].Name)
	assert.False(t, params[1].Required)
}

func TestConfigDeleteTool_RequiresPermission(t *testing.T) {
	tool := NewConfigDeleteTool(&schema.AtmosConfiguration{})
	assert.True(t, tool.RequiresPermission())
}

func TestConfigDeleteTool_IsRestricted(t *testing.T) {
	tool := NewConfigDeleteTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestConfigDeleteTool_Execute(t *testing.T) {
	tool := NewConfigDeleteTool(&schema.AtmosConfiguration{})
	ctx := context.Background()

	t.Run("deletes an existing value", func(t *testing.T) {
		dir := t.TempDir()
		file := writeConfigFixture(t, dir, "settings:\n  enabled: false\n  stale: yes\n")

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path": "settings.stale",
			"file": file,
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, true, result.Data["deleted"])
		assert.Contains(t, result.Output, "Deleted")

		_, getErr := atmosyaml.GetFile(file, "settings.stale")
		assert.ErrorIs(t, getErr, atmosyaml.ErrYAMLPathNotFound)

		// The rest of the file must survive untouched.
		got, getErr := atmosyaml.GetFile(file, "settings.enabled")
		require.NoError(t, getErr)
		assert.Equal(t, "false", got)
	})

	t.Run("no-op when path is already absent", func(t *testing.T) {
		dir := t.TempDir()
		file := writeConfigFixture(t, dir, "settings:\n  enabled: false\n")

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path": "settings.does_not_exist",
			"file": file,
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, false, result.Data["deleted"])
		assert.Contains(t, result.Output, "Nothing to delete")
	})

	t.Run("fails with missing path parameter", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"file": filepath.Join(t.TempDir(), "atmos.yaml"),
		})

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})

	t.Run("fails when the explicit file override does not exist", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"path": "settings.enabled",
			"file": filepath.Join(t.TempDir(), "does-not-exist.yaml"),
		})

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIConfigFileNotFound)
	})

	t.Run("fails with malformed path expression", func(t *testing.T) {
		dir := t.TempDir()
		file := writeConfigFixture(t, dir, "settings:\n  enabled: false\n")

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path": "a..b",
			"file": file,
		})

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, atmosyaml.ErrInvalidYAMLExpression)
	})
}
