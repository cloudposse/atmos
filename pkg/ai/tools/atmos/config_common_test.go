package atmos

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRequireStringParam(t *testing.T) {
	t.Run("returns value when present and non-empty", func(t *testing.T) {
		got, err := requireStringParam(map[string]interface{}{"path": "mcp.enabled"}, "path")
		require.NoError(t, err)
		assert.Equal(t, "mcp.enabled", got)
	})

	t.Run("errors when missing", func(t *testing.T) {
		_, err := requireStringParam(map[string]interface{}{}, "path")
		require.Error(t, err)
	})

	t.Run("errors when empty string", func(t *testing.T) {
		_, err := requireStringParam(map[string]interface{}{"path": ""}, "path")
		require.Error(t, err)
	})

	t.Run("errors when not a string", func(t *testing.T) {
		_, err := requireStringParam(map[string]interface{}{"path": 5}, "path")
		require.Error(t, err)
	})
}

func TestOptionalStringParam(t *testing.T) {
	t.Run("returns value when present", func(t *testing.T) {
		got := optionalStringParam(map[string]interface{}{"file": "atmos.yaml"}, "file")
		assert.Equal(t, "atmos.yaml", got)
	})

	t.Run("returns empty string when missing", func(t *testing.T) {
		got := optionalStringParam(map[string]interface{}{}, "file")
		assert.Empty(t, got)
	})

	t.Run("returns empty string when not a string", func(t *testing.T) {
		got := optionalStringParam(map[string]interface{}{"file": 5}, "file")
		assert.Empty(t, got)
	})
}

func TestResolveConfigTargetFile(t *testing.T) {
	t.Run("uses explicit file override", func(t *testing.T) {
		dir := t.TempDir()
		file := writeConfigFixture(t, dir, "logs:\n  level: info\n")

		got, err := resolveConfigTargetFile(&schema.AtmosConfiguration{}, map[string]interface{}{"file": file})
		require.NoError(t, err)
		assert.Equal(t, file, got)
	})

	t.Run("errors with errUtils.ErrAIConfigFileNotFound when override does not exist", func(t *testing.T) {
		_, err := resolveConfigTargetFile(&schema.AtmosConfiguration{}, map[string]interface{}{
			"file": filepath.Join(t.TempDir(), "missing.yaml"),
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrAIConfigFileNotFound)
		assert.ErrorIs(t, err, cfg.ErrNoEditableConfig)
	})

	t.Run("discovers atmos.yaml from the current directory when no override is given", func(t *testing.T) {
		dir := t.TempDir()
		file := writeConfigFixture(t, dir, "logs:\n  level: info\n")

		wd, err := os.Getwd()
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, os.Chdir(wd))
		})
		require.NoError(t, os.Chdir(dir))

		got, err := resolveConfigTargetFile(&schema.AtmosConfiguration{}, map[string]interface{}{})
		require.NoError(t, err)

		wantResolved, err := filepath.EvalSymlinks(file)
		require.NoError(t, err)
		gotResolved, err := filepath.EvalSymlinks(got)
		require.NoError(t, err)
		assert.Equal(t, wantResolved, gotResolved)
	})

	t.Run("errors with errUtils.ErrAIConfigFileNotFound when nothing can be discovered", func(t *testing.T) {
		dir := t.TempDir()

		wd, err := os.Getwd()
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, os.Chdir(wd))
		})
		require.NoError(t, os.Chdir(dir))

		_, err = resolveConfigTargetFile(&schema.AtmosConfiguration{}, map[string]interface{}{})
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrAIConfigFileNotFound)
	})
}

func TestConfigPathPatternRegexp(t *testing.T) {
	t.Run("wildcard matches prefix", func(t *testing.T) {
		re := configPathPatternRegexp("mcp.*")
		assert.True(t, re.MatchString("mcp.enabled"))
		assert.False(t, re.MatchString("logs.level"))
	})

	t.Run("question mark matches single character", func(t *testing.T) {
		re := configPathPatternRegexp("mcp.?")
		assert.True(t, re.MatchString("mcp.a"))
		assert.False(t, re.MatchString("mcp.ab"))
	})

	t.Run("exact pattern anchors the full string", func(t *testing.T) {
		re := configPathPatternRegexp("mcp.enabled")
		assert.True(t, re.MatchString("mcp.enabled"))
		assert.False(t, re.MatchString("mcp.enabled.extra"))
	})
}
