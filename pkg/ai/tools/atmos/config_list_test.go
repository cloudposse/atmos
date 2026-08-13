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

func TestNewConfigListTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewConfigListTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Equal(t, atmosConfig, tool.atmosConfig)
}

func TestConfigListTool_Name(t *testing.T) {
	tool := NewConfigListTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_config_list", tool.Name())
}

func TestConfigListTool_Description(t *testing.T) {
	tool := NewConfigListTool(&schema.AtmosConfiguration{})
	assert.Contains(t, tool.Description(), "List the dot-notation setting paths")
}

func TestConfigListTool_Parameters(t *testing.T) {
	tool := NewConfigListTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 2)
	assert.Equal(t, "pattern", params[0].Name)
	assert.False(t, params[0].Required)
	assert.Equal(t, "file", params[1].Name)
	assert.False(t, params[1].Required)
}

func TestConfigListTool_RequiresPermission(t *testing.T) {
	tool := NewConfigListTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.RequiresPermission())
}

func TestConfigListTool_IsRestricted(t *testing.T) {
	tool := NewConfigListTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestConfigListTool_Execute(t *testing.T) {
	tool := NewConfigListTool(&schema.AtmosConfiguration{})
	ctx := context.Background()

	t.Run("lists all paths with no pattern", func(t *testing.T) {
		dir := t.TempDir()
		file := writeConfigFixture(t, dir, "logs:\n  level: debug\nmcp:\n  enabled: true\n")

		result, err := tool.Execute(ctx, map[string]interface{}{
			"file": file,
		})

		require.NoError(t, err)
		assert.True(t, result.Success)

		entries, ok := result.Data["entries"].([]map[string]interface{})
		require.True(t, ok)
		assert.Len(t, entries, 4) // logs, logs.level, mcp, mcp.enabled

		var paths []string
		for _, e := range entries {
			paths = append(paths, e["path"].(string))
		}
		assert.Contains(t, paths, "logs.level")
		assert.Contains(t, paths, "mcp.enabled")
	})

	t.Run("filters by glob pattern", func(t *testing.T) {
		dir := t.TempDir()
		file := writeConfigFixture(t, dir, "logs:\n  level: debug\nmcp:\n  enabled: true\n")

		result, err := tool.Execute(ctx, map[string]interface{}{
			"pattern": "mcp.*",
			"file":    file,
		})

		require.NoError(t, err)
		assert.True(t, result.Success)

		entries, ok := result.Data["entries"].([]map[string]interface{})
		require.True(t, ok)
		require.Len(t, entries, 1)
		assert.Equal(t, "mcp.enabled", entries[0]["path"])
		assert.Equal(t, "bool", entries[0]["type"])
		assert.Equal(t, "true", entries[0]["value"])
	})

	t.Run("pattern matching nothing returns empty entries", func(t *testing.T) {
		dir := t.TempDir()
		file := writeConfigFixture(t, dir, "logs:\n  level: debug\n")

		result, err := tool.Execute(ctx, map[string]interface{}{
			"pattern": "no.such.path",
			"file":    file,
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		entries, ok := result.Data["entries"].([]map[string]interface{})
		require.True(t, ok)
		assert.Empty(t, entries)
	})

	t.Run("empty file yields no entries", func(t *testing.T) {
		dir := t.TempDir()
		file := writeConfigFixture(t, dir, "")

		result, err := tool.Execute(ctx, map[string]interface{}{
			"file": file,
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		entries, ok := result.Data["entries"].([]map[string]interface{})
		require.True(t, ok)
		assert.Empty(t, entries)
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
		file := writeConfigFixture(t, dir, "logs: {level: debug\n")

		result, err := tool.Execute(ctx, map[string]interface{}{
			"file": file,
		})

		require.Error(t, err)
		assert.False(t, result.Success)
	})
}

func TestFilterConfigPathEntries(t *testing.T) {
	entries := []atmosyaml.PathEntry{
		{Path: "logs.level", Type: "string", Value: "debug"},
		{Path: "mcp.enabled", Type: "bool", Value: "true"},
		{Path: "mcp.timeout", Type: "number", Value: "30"},
	}

	t.Run("empty pattern returns all entries", func(t *testing.T) {
		got := filterConfigPathEntries(entries, "")
		assert.Len(t, got, 3)
	})

	t.Run("wildcard prefix pattern", func(t *testing.T) {
		got := filterConfigPathEntries(entries, "mcp.*")
		assert.Len(t, got, 2)
		assert.Equal(t, "mcp.enabled", got[0].Path)
		assert.Equal(t, "mcp.timeout", got[1].Path)
	})

	t.Run("exact match pattern", func(t *testing.T) {
		got := filterConfigPathEntries(entries, "logs.level")
		require.Len(t, got, 1)
		assert.Equal(t, "logs.level", got[0].Path)
	})

	t.Run("no match", func(t *testing.T) {
		got := filterConfigPathEntries(entries, "nothing.matches")
		assert.Empty(t, got)
	})
}
