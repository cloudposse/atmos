package atmos

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version/manager"
)

// VersionTrackStatusTool reports whether tracked dependencies are locked,
// current, or have an update available.
type VersionTrackStatusTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewVersionTrackStatusTool creates a new version track status tool.
func NewVersionTrackStatusTool(atmosConfig *schema.AtmosConfiguration) *VersionTrackStatusTool {
	return &VersionTrackStatusTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *VersionTrackStatusTool) Name() string {
	return "atmos_version_track_status"
}

// Description returns the tool description.
func (t *VersionTrackStatusTool) Description() string {
	return "Check every dependency in an Atmos version track against its datasource: reports whether each is " +
		"unlocked, locked, current, has an update available, or has a newer version blocked by policy " +
		"(strategy cap or cooldown). Read-only -- never modifies the lock file."
}

// Parameters returns the tool parameters.
func (t *VersionTrackStatusTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        paramTrack,
			Description: "Version track to check. Omit to use the project's default track.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        paramGroup,
			Description: "Limit the check to entries in this group.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
}

// Execute checks every entry's status without mutating the lock file.
func (t *VersionTrackStatusTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	track, _ := params[paramTrack].(string)
	group, _ := params[paramGroup].(string)

	status, err := manager.StatusTrackWithContext(ctx, t.atmosConfig, track, group)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	var out strings.Builder
	fmt.Fprintf(&out, "Version track status (%s):\n\n", status.Track)
	for i := range status.Entries {
		e := &status.Entries[i]
		line := fmt.Sprintf("  - %s: %s", e.Name, e.Status)
		if e.Locked != "" || e.Resolved != "" {
			line += fmt.Sprintf(" (locked: %s, resolved: %s)", e.Locked, e.Resolved)
		}
		if e.Message != "" {
			line += fmt.Sprintf(" -- %s", e.Message)
		}
		out.WriteString(line + "\n")
	}
	if len(status.Entries) == 0 {
		out.WriteString("  (no dependencies configured)\n")
	}

	return &tools.Result{
		Success: true,
		Output:  out.String(),
		Data: map[string]interface{}{
			paramTrack: status.Track,
			"entries":  status.Entries,
		},
	}, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *VersionTrackStatusTool) RequiresPermission() bool {
	return false // Read-only operation, never writes.
}

// IsRestricted returns true if this tool is always restricted.
func (t *VersionTrackStatusTool) IsRestricted() bool {
	return false
}
