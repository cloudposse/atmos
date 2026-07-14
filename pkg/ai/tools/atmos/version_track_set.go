package atmos

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version/manager"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// VersionTrackSetTool updates fields of an existing version-track dependency entry.
type VersionTrackSetTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewVersionTrackSetTool creates a new version track set tool.
func NewVersionTrackSetTool(atmosConfig *schema.AtmosConfiguration) *VersionTrackSetTool {
	return &VersionTrackSetTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *VersionTrackSetTool) Name() string {
	return "atmos_version_track_set"
}

// Description returns the tool description.
func (t *VersionTrackSetTool) Description() string {
	return "Update fields of an existing dependency entry in an Atmos version track. Only the fields provided " +
		"are changed; everything else is left as-is. The entry must already exist -- use " +
		"atmos_version_track_add to create one. Requires user confirmation."
}

// Parameters returns the tool parameters.
func (t *VersionTrackSetTool) Parameters() []tools.Parameter {
	params := []tools.Parameter{
		{
			Name:        paramName,
			Description: "Dependency name/key within the track.",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        paramTrack,
			Description: "Version track the entry belongs to. Omit to use the project's default track.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        paramPackage,
			Description: "New upstream package identifier.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        paramProvider,
			Description: "New provider used to resolve the datasource.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        paramDesired,
			Description: "New desired version expression: a concrete version, a SemVer constraint, or 'latest'.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
	return append(params, versionUpdatePolicyParams(
		"New group name for shared update policy.",
		"New update pin mode.",
		"New list of version patterns to include when resolving updates (replaces the existing list).",
		"New list of version patterns to exclude when resolving updates (replaces the existing list).",
		"New prerelease-allowed setting.",
	)...)
}

// Execute updates the given fields of the dependency entry.
func (t *VersionTrackSetTool) Execute(_ context.Context, params map[string]interface{}) (*tools.Result, error) {
	name, err := extractRequiredStringParam(params, paramName)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}
	track, _ := params[paramTrack].(string)
	fields := buildVersionEntryFieldUpdates(params)
	if len(fields) == 0 {
		err := fmt.Errorf("%w: at least one field to update", errUtils.ErrAIToolParameterRequired)
		return &tools.Result{Success: false, Error: err}, err
	}

	file, err := manager.SetEntryFields(t.atmosConfig, track, name, fields)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	effectiveTrack := manager.EffectiveTrack(t.atmosConfig, track)
	output := fmt.Sprintf("Updated dependency %s in track %s (%s)", name, effectiveTrack, atmosyaml.DisplayPath(file))
	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			paramName:  name,
			paramTrack: effectiveTrack,
			"file":     atmosyaml.DisplayPath(file),
			"fields":   fields,
		},
	}, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *VersionTrackSetTool) RequiresPermission() bool {
	return true // Writing atmos.yaml requires confirmation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *VersionTrackSetTool) IsRestricted() bool {
	return false // User can allow via configuration.
}
