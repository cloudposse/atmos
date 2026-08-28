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

func TestNewVendorConfigListTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewVendorConfigListTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Equal(t, atmosConfig, tool.atmosConfig)
}

func TestVendorConfigListTool_Name(t *testing.T) {
	tool := NewVendorConfigListTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_vendor_config_list", tool.Name())
}

func TestVendorConfigListTool_Description(t *testing.T) {
	tool := NewVendorConfigListTool(&schema.AtmosConfiguration{})
	assert.Contains(t, tool.Description(), "vendor manifest")
}

func TestVendorConfigListTool_Parameters(t *testing.T) {
	tool := NewVendorConfigListTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 2)
	assert.Equal(t, "pattern", params[0].Name)
	assert.False(t, params[0].Required)
	assert.Equal(t, "file", params[1].Name)
	assert.False(t, params[1].Required)
}

func TestVendorConfigListTool_RequiresPermission(t *testing.T) {
	tool := NewVendorConfigListTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.RequiresPermission())
}

func TestVendorConfigListTool_IsRestricted(t *testing.T) {
	tool := NewVendorConfigListTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestVendorConfigListTool_Execute(t *testing.T) {
	tool := NewVendorConfigListTool(&schema.AtmosConfiguration{})
	ctx := context.Background()

	t.Run("lists entries across the root manifest and its imports, tagged by source file", func(t *testing.T) {
		dir := t.TempDir()
		rootFile, importedFile := writeVendorConfigFixtureWithImport(t, dir)

		result, err := tool.Execute(ctx, map[string]interface{}{
			"file": rootFile,
		})

		require.NoError(t, err)
		assert.True(t, result.Success)

		entries, ok := result.Data["entries"].([]map[string]interface{})
		require.True(t, ok)
		require.NotEmpty(t, entries)

		var sawRootEntry, sawImportedEntry bool
		for _, entry := range entries {
			switch entry["file"] {
			case rootFile:
				if entry["path"] == "spec.sources[0].component" {
					sawRootEntry = true
					assert.Equal(t, "vpc", entry["value"])
				}
			case importedFile:
				if entry["path"] == "spec.sources[0].component" {
					sawImportedEntry = true
					assert.Equal(t, "eks", entry["value"])
				}
			}
		}
		assert.True(t, sawRootEntry, "expected an entry tagged with the root file")
		assert.True(t, sawImportedEntry, "expected an entry tagged with the imported file")
	})

	t.Run("filters entries by glob pattern", func(t *testing.T) {
		dir := t.TempDir()
		rootFile, _ := writeVendorConfigFixtureWithImport(t, dir)

		result, err := tool.Execute(ctx, map[string]interface{}{
			"file":    rootFile,
			"pattern": "spec.sources[*].version",
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, "spec.sources[*].version", result.Data["pattern"])

		entries, ok := result.Data["entries"].([]map[string]interface{})
		require.True(t, ok)
		require.NotEmpty(t, entries)
		for _, entry := range entries {
			assert.Contains(t, entry["path"], "version")
		}
	})

	t.Run("lists using the default ./vendor.yaml", func(t *testing.T) {
		dir := t.TempDir()
		writeVendorConfigFixture(t, dir)
		t.Chdir(dir)

		result, err := tool.Execute(ctx, map[string]interface{}{})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, defaultVendorManifest, result.Data["file"])

		entries, ok := result.Data["entries"].([]map[string]interface{})
		require.True(t, ok)
		assert.NotEmpty(t, entries)
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
