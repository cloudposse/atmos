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

func TestNewConfigSetTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewConfigSetTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Equal(t, atmosConfig, tool.atmosConfig)
}

func TestConfigSetTool_Name(t *testing.T) {
	tool := NewConfigSetTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_config_set", tool.Name())
}

func TestConfigSetTool_Description(t *testing.T) {
	tool := NewConfigSetTool(&schema.AtmosConfiguration{})
	assert.Contains(t, tool.Description(), "Set a value")
}

func TestConfigSetTool_Parameters(t *testing.T) {
	tool := NewConfigSetTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 4)
	assert.Equal(t, "path", params[0].Name)
	assert.True(t, params[0].Required)
	assert.Equal(t, "value", params[1].Name)
	assert.True(t, params[1].Required)
	assert.Equal(t, "type", params[2].Name)
	assert.False(t, params[2].Required)
	assert.Equal(t, "file", params[3].Name)
	assert.False(t, params[3].Required)
}

func TestConfigSetTool_RequiresPermission(t *testing.T) {
	tool := NewConfigSetTool(&schema.AtmosConfiguration{})
	assert.True(t, tool.RequiresPermission())
}

func TestConfigSetTool_IsRestricted(t *testing.T) {
	tool := NewConfigSetTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestConfigSetTool_Execute(t *testing.T) {
	tool := NewConfigSetTool(&schema.AtmosConfiguration{})
	ctx := context.Background()

	t.Run("creates a new value and infers bool type from schema", func(t *testing.T) {
		dir := t.TempDir()
		file := writeConfigFixture(t, dir, "logs:\n  level: info\n")

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path":  "mcp.enabled",
			"value": "true",
			"file":  file,
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, true, result.Data["created"])
		assert.Equal(t, atmosyaml.TypeBool, result.Data["type"])
		assert.Contains(t, result.Output, "Created")

		got, getErr := atmosyaml.GetFile(file, "mcp.enabled")
		require.NoError(t, getErr)
		assert.Equal(t, "true", got)
	})

	t.Run("updates an existing value", func(t *testing.T) {
		dir := t.TempDir()
		file := writeConfigFixture(t, dir, "logs:\n  level: info\n")

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path":  "logs.level",
			"value": "debug",
			"file":  file,
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, false, result.Data["created"])
		assert.Contains(t, result.Output, "Updated")

		got, getErr := atmosyaml.GetFile(file, "logs.level")
		require.NoError(t, getErr)
		assert.Equal(t, "debug", got)
	})

	t.Run("falls back to string type for unmodeled paths", func(t *testing.T) {
		dir := t.TempDir()
		file := writeConfigFixture(t, dir, "vars:\n  region: us-east-1\n")

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path":  "vars.custom_free_form_key",
			"value": "some-value",
			"file":  file,
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, atmosyaml.TypeString, result.Data["type"])
	})

	t.Run("honors explicit type override", func(t *testing.T) {
		dir := t.TempDir()
		file := writeConfigFixture(t, dir, "settings:\n  count: 1\n")

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path":  "settings.count",
			"value": "42",
			"type":  atmosyaml.TypeInt,
			"file":  file,
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, atmosyaml.TypeInt, result.Data["type"])

		got, getErr := atmosyaml.GetFile(file, "settings.count")
		require.NoError(t, getErr)
		assert.Equal(t, "42", got)
	})

	t.Run("fails with missing path parameter", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"value": "true",
			"file":  filepath.Join(t.TempDir(), "atmos.yaml"),
		})

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})

	t.Run("fails with missing value parameter", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"path": "mcp.enabled",
			"file": filepath.Join(t.TempDir(), "atmos.yaml"),
		})

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})

	t.Run("fails when the explicit file override does not exist", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"path":  "mcp.enabled",
			"value": "true",
			"file":  filepath.Join(t.TempDir(), "does-not-exist.yaml"),
		})

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIConfigFileNotFound)
	})

	t.Run("fails when the explicit type does not validate the value", func(t *testing.T) {
		dir := t.TempDir()
		file := writeConfigFixture(t, dir, "settings:\n  count: 1\n")

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path":  "settings.count",
			"value": "not-an-int",
			"type":  atmosyaml.TypeInt,
			"file":  file,
		})

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, atmosyaml.ErrInvalidYAMLExpression)
	})
}

func TestResolveConfigSetValueType(t *testing.T) {
	t.Run("explicit type wins over inference", func(t *testing.T) {
		got := resolveConfigSetValueType(map[string]interface{}{"type": atmosyaml.TypeYAML}, "mcp.enabled")
		assert.Equal(t, atmosyaml.TypeYAML, got)
	})

	t.Run("infers bool from schema when type omitted", func(t *testing.T) {
		got := resolveConfigSetValueType(map[string]interface{}{}, "mcp.enabled")
		assert.Equal(t, atmosyaml.TypeBool, got)
	})

	t.Run("falls back to string for unmodeled paths", func(t *testing.T) {
		got := resolveConfigSetValueType(map[string]interface{}{}, "vars.custom_key")
		assert.Equal(t, atmosyaml.TypeString, got)
	})
}
