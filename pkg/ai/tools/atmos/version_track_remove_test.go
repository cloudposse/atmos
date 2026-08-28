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

func TestNewVersionTrackRemoveTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewVersionTrackRemoveTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Same(t, atmosConfig, tool.atmosConfig)
}

func TestVersionTrackRemoveTool_Name(t *testing.T) {
	tool := NewVersionTrackRemoveTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_version_track_remove", tool.Name())
}

func TestVersionTrackRemoveTool_Description(t *testing.T) {
	tool := NewVersionTrackRemoveTool(&schema.AtmosConfiguration{})
	assert.NotEmpty(t, tool.Description())
}

func TestVersionTrackRemoveTool_Parameters(t *testing.T) {
	tool := NewVersionTrackRemoveTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 2)
	assert.Equal(t, paramName, params[0].Name)
	assert.True(t, params[0].Required)
	assert.Equal(t, paramTrack, params[1].Name)
	assert.False(t, params[1].Required)
}

func TestVersionTrackRemoveTool_RequiresPermission(t *testing.T) {
	tool := NewVersionTrackRemoveTool(&schema.AtmosConfiguration{})
	assert.True(t, tool.RequiresPermission())
}

func TestVersionTrackRemoveTool_IsRestricted(t *testing.T) {
	tool := NewVersionTrackRemoveTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestVersionTrackRemoveTool_Execute(t *testing.T) {
	file := versionTrackSandbox(t)
	tool := NewVersionTrackRemoveTool(&schema.AtmosConfiguration{})
	ctx := context.Background()

	t.Run("removes an existing entry", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			paramName:  "opentofu",
			paramTrack: "prod",
		})
		require.NoError(t, err)
		require.True(t, result.Success)

		content, err := os.ReadFile(file)
		require.NoError(t, err)
		assert.NotContains(t, string(content), "opentofu")
		assert.Contains(t, string(content), "# Project configuration")
	})

	t.Run("fails for an entry that does not exist", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			paramName: "nonexistent",
		})
		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, manager.ErrEntryNotFound)
	})

	t.Run("fails with missing name", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{})
		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})
}
