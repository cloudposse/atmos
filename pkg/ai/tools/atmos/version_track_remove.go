package atmos

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version/manager"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// VersionTrackRemoveTool removes a dependency entry from a version track.
type VersionTrackRemoveTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewVersionTrackRemoveTool creates a new version track remove tool.
func NewVersionTrackRemoveTool(atmosConfig *schema.AtmosConfiguration) *VersionTrackRemoveTool {
	return &VersionTrackRemoveTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *VersionTrackRemoveTool) Name() string {
	return "atmos_version_track_remove"
}

// Description returns the tool description.
func (t *VersionTrackRemoveTool) Description() string {
	return "Remove a dependency entry from an Atmos version track. Requires user confirmation."
}

// Parameters returns the tool parameters.
func (t *VersionTrackRemoveTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        paramName,
			Description: "Dependency name/key to remove.",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        paramTrack,
			Description: "Version track the entry belongs to. Omit to use the project's default track.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
}

// Execute removes the dependency entry from the track.
func (t *VersionTrackRemoveTool) Execute(_ context.Context, params map[string]interface{}) (*tools.Result, error) {
	name, err := extractRequiredStringParam(params, paramName)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}
	track, _ := params[paramTrack].(string)

	file, err := manager.RemoveEntry(t.atmosConfig, track, name)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	effectiveTrack := manager.EffectiveTrack(t.atmosConfig, track)
	output := fmt.Sprintf("Removed dependency %s from track %s (%s)", name, effectiveTrack, atmosyaml.DisplayPath(file))
	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			paramName:  name,
			paramTrack: effectiveTrack,
			"file":     atmosyaml.DisplayPath(file),
		},
	}, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *VersionTrackRemoveTool) RequiresPermission() bool {
	return true // Writing atmos.yaml requires confirmation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *VersionTrackRemoveTool) IsRestricted() bool {
	return false // User can allow via configuration.
}
