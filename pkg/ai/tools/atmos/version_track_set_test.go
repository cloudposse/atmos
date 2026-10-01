package atmos

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version/manager"
)

func TestNewVersionTrackSetTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewVersionTrackSetTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Same(t, atmosConfig, tool.atmosConfig)
}

func TestVersionTrackSetTool_Name(t *testing.T) {
	tool := NewVersionTrackSetTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_version_track_set", tool.Name())
}

func TestVersionTrackSetTool_Description(t *testing.T) {
	tool := NewVersionTrackSetTool(&schema.AtmosConfiguration{})
	assert.NotEmpty(t, tool.Description())
}

func TestVersionTrackSetTool_Parameters(t *testing.T) {
	tool := NewVersionTrackSetTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 10)
	assert.Equal(t, paramName, params[0].Name)
	assert.True(t, params[0].Required)
}

func TestVersionTrackSetTool_RequiresPermission(t *testing.T) {
	tool := NewVersionTrackSetTool(&schema.AtmosConfiguration{})
	assert.True(t, tool.RequiresPermission())
}

func TestVersionTrackSetTool_IsRestricted(t *testing.T) {
	tool := NewVersionTrackSetTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestVersionTrackSetTool_Execute(t *testing.T) {
	file := versionTrackSandbox(t)
	tool := NewVersionTrackSetTool(&schema.AtmosConfiguration{})
	ctx := context.Background()

	t.Run("updates a field, preserving comments", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			paramName:    "opentofu",
			paramTrack:   "prod",
			paramDesired: "~1.11",
		})
		require.NoError(t, err)
		require.True(t, result.Success)

		content, err := os.ReadFile(file)
		require.NoError(t, err)
		s := string(content)
		assert.Contains(t, s, "# Keep opentofu on 1.10")
		assert.Contains(t, s, "~1.11")
	})

	t.Run("fails for an entry that does not exist", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			paramName:    "nonexistent",
			paramDesired: "1.0.0",
		})
		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, manager.ErrEntryNotFound)
	})

	t.Run("fails with no fields to update", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			paramName: "opentofu",
		})
		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})

	t.Run("fails with missing name", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			paramDesired: "1.0.0",
		})
		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})
}
