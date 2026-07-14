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

func TestNewVendorConfigSetTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewVendorConfigSetTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Equal(t, atmosConfig, tool.atmosConfig)
}

func TestVendorConfigSetTool_Name(t *testing.T) {
	tool := NewVendorConfigSetTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_vendor_config_set", tool.Name())
}

func TestVendorConfigSetTool_Description(t *testing.T) {
	tool := NewVendorConfigSetTool(&schema.AtmosConfiguration{})
	assert.Contains(t, tool.Description(), "vendor manifest")
}

func TestVendorConfigSetTool_Parameters(t *testing.T) {
	tool := NewVendorConfigSetTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 4)
	assert.Equal(t, "path", params[0].Name)
	assert.True(t, params[0].Required)
	assert.Equal(t, "value", params[1].Name)
	assert.True(t, params[1].Required)
	assert.Equal(t, "type", params[2].Name)
	assert.False(t, params[2].Required)
	assert.Equal(t, atmosyaml.TypeString, params[2].Default)
	assert.Equal(t, "file", params[3].Name)
	assert.False(t, params[3].Required)
}

func TestVendorConfigSetTool_RequiresPermission(t *testing.T) {
	tool := NewVendorConfigSetTool(&schema.AtmosConfiguration{})
	assert.True(t, tool.RequiresPermission())
}

func TestVendorConfigSetTool_IsRestricted(t *testing.T) {
	tool := NewVendorConfigSetTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestVendorConfigSetTool_Execute(t *testing.T) {
	tool := NewVendorConfigSetTool(&schema.AtmosConfiguration{})
	ctx := context.Background()

	t.Run("successfully updates an existing value", func(t *testing.T) {
		dir := t.TempDir()
		file := writeVendorConfigFixture(t, dir)

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path":  "spec.sources[0].version",
			"value": "v1.2.3",
			"file":  file,
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Contains(t, result.Output, "v1.2.3")
		assert.Equal(t, "v1.2.3", result.Data["value"])
		assert.False(t, result.Data["created"].(bool))

		value, err := atmosyaml.GetFile(file, "spec.sources[0].version")
		require.NoError(t, err)
		assert.Equal(t, "v1.2.3", value)
	})

	t.Run("successfully creates a new value and echoes it", func(t *testing.T) {
		dir := t.TempDir()
		file := writeVendorConfigFixture(t, dir)

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path":  "spec.sources[0].newkey",
			"value": "hello",
			"file":  file,
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.True(t, result.Data["created"].(bool))
		assert.Contains(t, result.Output, "hello")

		value, err := atmosyaml.GetFile(file, "spec.sources[0].newkey")
		require.NoError(t, err)
		assert.Equal(t, "hello", value)
	})

	t.Run("successfully sets a typed value using the default ./vendor.yaml", func(t *testing.T) {
		dir := t.TempDir()
		writeVendorConfigFixture(t, dir)
		t.Chdir(dir)

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path":  "spec.sources[0].pinned",
			"value": "true",
			"type":  atmosyaml.TypeBool,
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, atmosyaml.TypeBool, result.Data["type"])

		value, err := atmosyaml.GetFile(defaultVendorManifest, "spec.sources[0].pinned")
		require.NoError(t, err)
		assert.Equal(t, "true", value)
	})

	t.Run("fails with missing path", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"value": "v1",
		})

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})

	t.Run("fails with missing value", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"path": "spec.sources[0].version",
		})

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})

	t.Run("fails when vendor manifest does not exist", func(t *testing.T) {
		t.Chdir(t.TempDir())

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path":  "spec.sources[0].version",
			"value": "v1",
		})

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIVendorFileNotFound)
	})

	t.Run("fails with an invalid typed value", func(t *testing.T) {
		dir := t.TempDir()
		file := writeVendorConfigFixture(t, dir)

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path":  "spec.sources[0].pinned",
			"value": "not-a-bool",
			"type":  atmosyaml.TypeBool,
			"file":  file,
		})

		require.Error(t, err)
		assert.False(t, result.Success)
	})

	t.Run("fails with malformed manifest content", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "vendor.yaml")
		require.NoError(t, os.WriteFile(file, []byte("spec: ["), 0o600))

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path":  "spec.sources[0].version",
			"value": "v1",
			"file":  file,
		})

		require.Error(t, err)
		assert.False(t, result.Success)
	})
}
