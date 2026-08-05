package atmos

import (
	"context"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
)

// VendorUpdateTool advances vendored Git-sourced components' pinned versions
// to the latest available tag, writing the new version back to the manifest
// that declares them. It never runs `vendor pull` itself.
type VendorUpdateTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewVendorUpdateTool creates a new vendor update tool.
func NewVendorUpdateTool(atmosConfig *schema.AtmosConfiguration) *VendorUpdateTool {
	return &VendorUpdateTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *VendorUpdateTool) Name() string {
	return "atmos_vendor_update"
}

// Description returns the tool description.
func (t *VendorUpdateTool) Description() string {
	return "Update vendored Git-sourced components' pinned version to the latest available tag, writing the " +
		"new version back to the manifest that declares them (vendor.yaml or component.yaml). Given a " +
		"component, updates just that one; otherwise updates every Git-sourced component declared in the " +
		"project's vendor.yaml. Does not download/pull the new source -- run atmos vendor pull separately " +
		"after reviewing the version change. Requires user confirmation."
}

// Parameters returns the tool parameters.
func (t *VendorUpdateTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        paramComponent,
			Description: "Component name to update (e.g. 'vpc'). Omit to update every source in vendor.yaml.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        paramTags,
			Description: "Limit the update to sources tagged with any of these tags (only applies to the repo-wide scan).",
			Type:        tools.ParamTypeArray,
			Required:    false,
		},
	}
}

// Execute advances pinned versions to the latest available tag.
func (t *VendorUpdateTool) Execute(_ context.Context, params map[string]interface{}) (*tools.Result, error) {
	component, _ := params[paramComponent].(string)
	tags := extractStringSliceParam(params, paramTags)

	report, err := runVendorUpdateCheck(component, tags, false)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	return buildVendorUpdateResult(report, false), nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *VendorUpdateTool) RequiresPermission() bool {
	return true // Writing manifest version pins requires confirmation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *VendorUpdateTool) IsRestricted() bool {
	return false // User can allow via configuration.
}
