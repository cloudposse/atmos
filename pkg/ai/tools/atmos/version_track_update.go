package atmos

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version/manager"
)

// VersionTrackUpdateTool advances locked versions within each entry's update
// policy and writes the result to the track's lock file.
type VersionTrackUpdateTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewVersionTrackUpdateTool creates a new version track update tool.
func NewVersionTrackUpdateTool(atmosConfig *schema.AtmosConfiguration) *VersionTrackUpdateTool {
	return &VersionTrackUpdateTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *VersionTrackUpdateTool) Name() string {
	return "atmos_version_track_update"
}

// Description returns the tool description.
func (t *VersionTrackUpdateTool) Description() string {
	return "Advance a version track's locked versions within each entry's update policy (strategy caps, " +
		"cooldown, include/exclude, prerelease rules) and write the result to the lock file. Held-back " +
		"updates report a reason. Requires user confirmation."
}

// Parameters returns the tool parameters.
func (t *VersionTrackUpdateTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        paramTrack,
			Description: "Version track to update. Omit to use the project's default track.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        paramGroup,
			Description: "Limit the update to entries in this group.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        "only",
			Description: "Limit the update to these specific dependency names.",
			Type:        tools.ParamTypeArray,
			Required:    false,
		},
	}
}

// Execute advances locked versions and writes the lock file.
func (t *VersionTrackUpdateTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	track, _ := params[paramTrack].(string)
	group, _ := params[paramGroup].(string)
	only := extractStringSliceParam(params, "only")

	update, err := manager.UpdateTrackWithContext(ctx, t.atmosConfig, track, group, only)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	var out strings.Builder
	fmt.Fprintf(&out, "Version track update (%s):\n\n", update.Track)
	updatedCount := 0
	for _, r := range update.Results {
		status := "unchanged"
		if r.Updated {
			status = "updated"
			updatedCount++
		}
		line := fmt.Sprintf("  - %s: %s", r.Name, status)
		if r.From != "" || r.To != "" {
			line += fmt.Sprintf(" (%s -> %s)", r.From, r.To)
		}
		if r.Reason != "" {
			line += fmt.Sprintf(" -- %s", r.Reason)
		}
		out.WriteString(line + "\n")
	}
	if len(update.Results) == 0 {
		out.WriteString("  (no dependencies configured)\n")
	}

	return &tools.Result{
		Success: true,
		Output:  out.String(),
		Data: map[string]interface{}{
			paramTrack:      update.Track,
			"updated_count": updatedCount,
			"results":       update.Results,
		},
	}, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *VersionTrackUpdateTool) RequiresPermission() bool {
	return true // Writes the lock file.
}

// IsRestricted returns true if this tool is always restricted.
func (t *VersionTrackUpdateTool) IsRestricted() bool {
	return false // User can allow via configuration.
}
