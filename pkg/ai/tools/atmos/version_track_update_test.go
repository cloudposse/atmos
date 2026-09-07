package atmos

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewVersionTrackUpdateTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewVersionTrackUpdateTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Same(t, atmosConfig, tool.atmosConfig)
}

func TestVersionTrackUpdateTool_Name(t *testing.T) {
	tool := NewVersionTrackUpdateTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_version_track_update", tool.Name())
}

func TestVersionTrackUpdateTool_Description(t *testing.T) {
	tool := NewVersionTrackUpdateTool(&schema.AtmosConfiguration{})
	assert.NotEmpty(t, tool.Description())
}

func TestVersionTrackUpdateTool_Parameters(t *testing.T) {
	tool := NewVersionTrackUpdateTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 3)
	assert.Equal(t, paramTrack, params[0].Name)
	assert.Equal(t, paramGroup, params[1].Name)
	assert.Equal(t, "only", params[2].Name)
}

func TestVersionTrackUpdateTool_RequiresPermission(t *testing.T) {
	tool := NewVersionTrackUpdateTool(&schema.AtmosConfiguration{})
	assert.True(t, tool.RequiresPermission())
}

func TestVersionTrackUpdateTool_IsRestricted(t *testing.T) {
	tool := NewVersionTrackUpdateTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestVersionTrackUpdateTool_Execute(t *testing.T) {
	atmosConfig := versionTrackFakeConfig(t)
	tool := NewVersionTrackUpdateTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramTrack: "prod",
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	assert.Contains(t, result.Output, "thing")
	assert.Equal(t, "prod", result.Data[paramTrack])
	assert.Contains(t, result.Data, "updated_count")
}
