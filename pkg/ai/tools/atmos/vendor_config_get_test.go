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

func TestNewVendorConfigGetTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewVendorConfigGetTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Equal(t, atmosConfig, tool.atmosConfig)
}

func TestVendorConfigGetTool_Name(t *testing.T) {
	tool := NewVendorConfigGetTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_vendor_config_get", tool.Name())
}

func TestVendorConfigGetTool_Description(t *testing.T) {
	tool := NewVendorConfigGetTool(&schema.AtmosConfiguration{})
	assert.Contains(t, tool.Description(), "vendor manifest")
}

func TestVendorConfigGetTool_Parameters(t *testing.T) {
	tool := NewVendorConfigGetTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 2)
	assert.Equal(t, "path", params[0].Name)
	assert.True(t, params[0].Required)
	assert.Equal(t, "file", params[1].Name)
	assert.False(t, params[1].Required)
}

func TestVendorConfigGetTool_RequiresPermission(t *testing.T) {
	tool := NewVendorConfigGetTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.RequiresPermission())
}

func TestVendorConfigGetTool_IsRestricted(t *testing.T) {
	tool := NewVendorConfigGetTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestVendorConfigGetTool_Execute(t *testing.T) {
	tool := NewVendorConfigGetTool(&schema.AtmosConfiguration{})
	ctx := context.Background()

	t.Run("successfully reads a value using the file param", func(t *testing.T) {
		dir := t.TempDir()
		file := writeVendorConfigFixture(t, dir)

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path": "spec.sources[0].version",
			"file": file,
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, "v0", result.Output)
		assert.Equal(t, "v0", result.Data["value"])
		assert.Equal(t, file, result.Data["file"])
	})

	t.Run("successfully reads a value using the default ./vendor.yaml", func(t *testing.T) {
		dir := t.TempDir()
		writeVendorConfigFixture(t, dir)
		t.Chdir(dir)

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path": "spec.sources[0].component",
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, "vpc", result.Output)
	})

	t.Run("fails with missing path", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{})

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})

	t.Run("fails when path not found in manifest", func(t *testing.T) {
		dir := t.TempDir()
		file := writeVendorConfigFixture(t, dir)

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path": "spec.sources[99].version",
			"file": file,
		})

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, atmosyaml.ErrYAMLPathNotFound)
	})

	t.Run("fails when vendor manifest does not exist", func(t *testing.T) {
		t.Chdir(t.TempDir())

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path": "spec.sources[0].version",
		})

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIVendorFileNotFound)
	})

	t.Run("fails with malformed manifest content", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "vendor.yaml")
		require.NoError(t, os.WriteFile(file, []byte("spec: ["), 0o600))

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path": "spec.sources[0].version",
			"file": file,
		})

		require.Error(t, err)
		assert.False(t, result.Success)
	})
}
