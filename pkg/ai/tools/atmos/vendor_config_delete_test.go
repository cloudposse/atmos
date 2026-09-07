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

func TestNewVendorConfigDeleteTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewVendorConfigDeleteTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Equal(t, atmosConfig, tool.atmosConfig)
}

func TestVendorConfigDeleteTool_Name(t *testing.T) {
	tool := NewVendorConfigDeleteTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_vendor_config_delete", tool.Name())
}

func TestVendorConfigDeleteTool_Description(t *testing.T) {
	tool := NewVendorConfigDeleteTool(&schema.AtmosConfiguration{})
	assert.Contains(t, tool.Description(), "vendor manifest")
}

func TestVendorConfigDeleteTool_Parameters(t *testing.T) {
	tool := NewVendorConfigDeleteTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 2)
	assert.Equal(t, "path", params[0].Name)
	assert.True(t, params[0].Required)
	assert.Equal(t, "file", params[1].Name)
	assert.False(t, params[1].Required)
}

func TestVendorConfigDeleteTool_RequiresPermission(t *testing.T) {
	tool := NewVendorConfigDeleteTool(&schema.AtmosConfiguration{})
	assert.True(t, tool.RequiresPermission())
}

func TestVendorConfigDeleteTool_IsRestricted(t *testing.T) {
	tool := NewVendorConfigDeleteTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestVendorConfigDeleteTool_Execute(t *testing.T) {
	tool := NewVendorConfigDeleteTool(&schema.AtmosConfiguration{})
	ctx := context.Background()

	t.Run("successfully deletes an existing value", func(t *testing.T) {
		dir := t.TempDir()
		file := writeVendorConfigFixture(t, dir)

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path": "spec.sources[0].tags",
			"file": file,
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.True(t, result.Data["deleted"].(bool))
		assert.Contains(t, result.Output, "Deleted")

		_, err = atmosyaml.GetFile(file, "spec.sources[0].tags")
		assert.ErrorIs(t, err, atmosyaml.ErrYAMLPathNotFound)
	})

	t.Run("reports nothing to delete when path is already absent", func(t *testing.T) {
		dir := t.TempDir()
		file := writeVendorConfigFixture(t, dir)

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path": "spec.sources[0].doesnotexist",
			"file": file,
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.False(t, result.Data["deleted"].(bool))
		assert.Contains(t, result.Output, "Nothing to delete")
	})

	t.Run("successfully deletes using the default ./vendor.yaml", func(t *testing.T) {
		dir := t.TempDir()
		writeVendorConfigFixture(t, dir)
		t.Chdir(dir)

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path": "spec.sources[0].targets",
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.True(t, result.Data["deleted"].(bool))
	})

	t.Run("fails with missing path", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{})

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
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
