package atmos

import (
	"context"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
)

// VendorCheckUpdatesTool reports whether vendored Git-sourced components have
// a newer version available than the one currently pinned, without mutating
// any files.
type VendorCheckUpdatesTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewVendorCheckUpdatesTool creates a new vendor check-updates tool.
func NewVendorCheckUpdatesTool(atmosConfig *schema.AtmosConfiguration) *VendorCheckUpdatesTool {
	return &VendorCheckUpdatesTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *VendorCheckUpdatesTool) Name() string {
	return "atmos_vendor_check_updates"
}

// Description returns the tool description.
func (t *VendorCheckUpdatesTool) Description() string {
	return "Check whether vendored Git-sourced components have a newer version available than the one " +
		"currently pinned. Given a component, checks just that one; otherwise checks every Git-sourced " +
		"component declared in the project's vendor.yaml. Read-only -- never modifies files."
}

// Parameters returns the tool parameters.
func (t *VendorCheckUpdatesTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        paramComponent,
			Description: "Component name to check (e.g. 'vpc'). Omit to check every source in vendor.yaml.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        paramTags,
			Description: "Limit the check to sources tagged with any of these tags (only applies to the repo-wide scan).",
			Type:        tools.ParamTypeArray,
			Required:    false,
		},
	}
}

// Execute checks for newer available versions without mutating any files.
func (t *VendorCheckUpdatesTool) Execute(_ context.Context, params map[string]interface{}) (*tools.Result, error) {
	component, _ := params[paramComponent].(string)
	tags := extractStringSliceParam(params, paramTags)

	report, err := runVendorUpdateCheck(component, tags, true)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	return buildVendorUpdateResult(report, true), nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *VendorCheckUpdatesTool) RequiresPermission() bool {
	return false // Read-only operation, never writes.
}

// IsRestricted returns true if this tool is always restricted.
func (t *VendorCheckUpdatesTool) IsRestricted() bool {
	return false
}
