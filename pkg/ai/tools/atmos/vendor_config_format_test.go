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
)

func TestNewVendorConfigFormatTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewVendorConfigFormatTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Equal(t, atmosConfig, tool.atmosConfig)
}

func TestVendorConfigFormatTool_Name(t *testing.T) {
	tool := NewVendorConfigFormatTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_vendor_config_format", tool.Name())
}

func TestVendorConfigFormatTool_Description(t *testing.T) {
	tool := NewVendorConfigFormatTool(&schema.AtmosConfiguration{})
	assert.Contains(t, tool.Description(), "vendor manifest")
}

func TestVendorConfigFormatTool_Parameters(t *testing.T) {
	tool := NewVendorConfigFormatTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 1)
	assert.Equal(t, "file", params[0].Name)
	assert.False(t, params[0].Required)
}

func TestVendorConfigFormatTool_RequiresPermission(t *testing.T) {
	tool := NewVendorConfigFormatTool(&schema.AtmosConfiguration{})
	assert.True(t, tool.RequiresPermission())
}

func TestVendorConfigFormatTool_IsRestricted(t *testing.T) {
	tool := NewVendorConfigFormatTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestVendorConfigFormatTool_Execute(t *testing.T) {
	tool := NewVendorConfigFormatTool(&schema.AtmosConfiguration{})
	ctx := context.Background()

	t.Run("successfully formats the target file only, not its imports", func(t *testing.T) {
		dir := t.TempDir()
		rootFile, importedFile := writeVendorConfigFixtureWithImport(t, dir)

		messyImported := "apiVersion: atmos/v1\nkind:   AtmosVendorConfig\nspec:\n  sources:\n     -    component:   \"eks\"\n          source: \"github.com/cloudposse/terraform-aws-eks\"\n          version: \"1.2.3\"\n"
		require.NoError(t, os.WriteFile(importedFile, []byte(messyImported), 0o600))
		before, err := os.ReadFile(importedFile)
		require.NoError(t, err)

		result, err := tool.Execute(ctx, map[string]interface{}{
			"file": rootFile,
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, rootFile, result.Data["file"])

		// The imported manifest must be untouched: format only targets the
		// resolved root file, matching `atmos vendor config format`'s
		// single-file behavior (not the whole import graph).
		after, err := os.ReadFile(importedFile)
		require.NoError(t, err)
		assert.Equal(t, string(before), string(after))
	})

	t.Run("successfully formats using the default ./vendor.yaml", func(t *testing.T) {
		dir := t.TempDir()
		writeVendorConfigFixture(t, dir)
		t.Chdir(dir)

		result, err := tool.Execute(ctx, map[string]interface{}{})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, defaultVendorManifest, result.Data["file"])
	})

	t.Run("fails when vendor manifest does not exist", func(t *testing.T) {
		t.Chdir(t.TempDir())

		result, err := tool.Execute(ctx, map[string]interface{}{})

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIVendorFileNotFound)
	})

	t.Run("fails with malformed manifest content", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "vendor.yaml")
		require.NoError(t, os.WriteFile(file, []byte("spec: ["), 0o600))

		result, err := tool.Execute(ctx, map[string]interface{}{
			"file": file,
		})

		require.Error(t, err)
		assert.False(t, result.Success)
	})
}
