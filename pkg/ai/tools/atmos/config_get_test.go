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

// writeConfigFixture writes an atmos.yaml file to dir and returns its path.
func writeConfigFixture(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestNewConfigGetTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewConfigGetTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Equal(t, atmosConfig, tool.atmosConfig)
}

func TestConfigGetTool_Name(t *testing.T) {
	tool := NewConfigGetTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_config_get", tool.Name())
}

func TestConfigGetTool_Description(t *testing.T) {
	tool := NewConfigGetTool(&schema.AtmosConfiguration{})
	assert.Contains(t, tool.Description(), "Read a value")
}

func TestConfigGetTool_Parameters(t *testing.T) {
	tool := NewConfigGetTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 2)
	assert.Equal(t, "path", params[0].Name)
	assert.True(t, params[0].Required)
	assert.Equal(t, "file", params[1].Name)
	assert.False(t, params[1].Required)
}

func TestConfigGetTool_RequiresPermission(t *testing.T) {
	tool := NewConfigGetTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.RequiresPermission())
}

func TestConfigGetTool_IsRestricted(t *testing.T) {
	tool := NewConfigGetTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestConfigGetTool_Execute(t *testing.T) {
	tool := NewConfigGetTool(&schema.AtmosConfiguration{})
	ctx := context.Background()

	t.Run("successfully reads a value with explicit file override", func(t *testing.T) {
		dir := t.TempDir()
		file := writeConfigFixture(t, dir, "logs:\n  level: debug\nmcp:\n  enabled: true\n")

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path": "logs.level",
			"file": file,
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Contains(t, result.Output, "logs.level = debug")
		assert.Equal(t, "logs.level", result.Data["path"])
		assert.Equal(t, "debug", result.Data["value"])
	})

	t.Run("reads a bool value", func(t *testing.T) {
		dir := t.TempDir()
		file := writeConfigFixture(t, dir, "mcp:\n  enabled: true\n")

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path": "mcp.enabled",
			"file": file,
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, "true", result.Data["value"])
	})

	t.Run("fails with missing path parameter", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"file": filepath.Join(t.TempDir(), "atmos.yaml"),
		})

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})

	t.Run("fails with empty path parameter", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"path": "",
			"file": filepath.Join(t.TempDir(), "atmos.yaml"),
		})

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})

	t.Run("fails when the explicit file override does not exist", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"path": "logs.level",
			"file": filepath.Join(t.TempDir(), "does-not-exist.yaml"),
		})

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIConfigFileNotFound)
	})

	t.Run("fails when the path does not exist in the file", func(t *testing.T) {
		dir := t.TempDir()
		file := writeConfigFixture(t, dir, "logs:\n  level: debug\n")

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path": "logs.does_not_exist",
			"file": file,
		})

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, atmosyaml.ErrYAMLPathNotFound)
	})

	t.Run("fails with malformed yaml path expression", func(t *testing.T) {
		dir := t.TempDir()
		file := writeConfigFixture(t, dir, "logs:\n  level: debug\n")

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path": "a..b",
			"file": file,
		})

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, atmosyaml.ErrInvalidYAMLExpression)
	})
}
