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

const versionTrackConfigFixture = `# Project configuration -- hand-written comment that must survive edits.
base_path: "."

version:
  track: prod
  tracks:
    prod:
      dependencies:
        # Keep opentofu on 1.10 until the provider matrix is validated.
        opentofu:
          ecosystem: toolchain
          package: opentofu
          desired: "~1.10"
`

// versionTrackSandbox writes the fixture atmos.yaml into a temp working
// directory and chdirs there so cfg.ResolveEditableConfigFile finds it,
// mirroring pkg/version/manager/crud_test.go's own crudSandbox helper.
func versionTrackSandbox(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(file, []byte(versionTrackConfigFixture), 0o600))
	t.Chdir(dir)
	return file
}

func TestNewVersionTrackAddTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewVersionTrackAddTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Same(t, atmosConfig, tool.atmosConfig)
}

func TestVersionTrackAddTool_Name(t *testing.T) {
	tool := NewVersionTrackAddTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_version_track_add", tool.Name())
}

func TestVersionTrackAddTool_Description(t *testing.T) {
	tool := NewVersionTrackAddTool(&schema.AtmosConfiguration{})
	assert.NotEmpty(t, tool.Description())
}

func TestVersionTrackAddTool_Parameters(t *testing.T) {
	tool := NewVersionTrackAddTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 12)
	assert.Equal(t, paramName, params[0].Name)
	assert.True(t, params[0].Required)
}

func TestVersionTrackAddTool_RequiresPermission(t *testing.T) {
	tool := NewVersionTrackAddTool(&schema.AtmosConfiguration{})
	assert.True(t, tool.RequiresPermission())
}

func TestVersionTrackAddTool_IsRestricted(t *testing.T) {
	tool := NewVersionTrackAddTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestVersionTrackAddTool_Execute(t *testing.T) {
	file := versionTrackSandbox(t)
	tool := NewVersionTrackAddTool(&schema.AtmosConfiguration{})
	ctx := context.Background()

	t.Run("adds a new dependency, preserving comments", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			paramName:      "actions/checkout",
			paramTrack:     "prod",
			paramEcosystem: "github/actions",
			paramDesired:   "v6",
			paramPin:       "sha",
		})
		require.NoError(t, err)
		require.True(t, result.Success)

		content, err := os.ReadFile(file)
		require.NoError(t, err)
		s := string(content)
		assert.Contains(t, s, "# Project configuration")
		assert.Contains(t, s, "# Keep opentofu on 1.10")
		assert.Contains(t, s, "actions/checkout")
		assert.Contains(t, s, "v6")
	})

	t.Run("fails on a duplicate entry", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			paramName:  "opentofu",
			paramTrack: "prod",
		})
		require.Error(t, err)
		assert.False(t, result.Success)
	})

	t.Run("fails with missing name", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{})
		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})
}
