package atmos

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring"
)

// VendorDiffTool shows a unified diff between a vendored Git-sourced
// component's current pinned version and another ref.
type VendorDiffTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewVendorDiffTool creates a new vendor diff tool.
func NewVendorDiffTool(atmosConfig *schema.AtmosConfiguration) *VendorDiffTool {
	return &VendorDiffTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *VendorDiffTool) Name() string {
	return "atmos_vendor_diff"
}

// Description returns the tool description.
func (t *VendorDiffTool) Description() string {
	return "Show a unified diff between a vendored Git-sourced component's current pinned version and another " +
		"ref, defaulting to the latest available semver tag. Read-only."
}

// Parameters returns the tool parameters.
func (t *VendorDiffTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        paramComponent,
			Description: "Component name to diff (e.g. 'vpc').",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        "from",
			Description: "Starting ref. Defaults to the component's current pinned version.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        "to",
			Description: "Ending ref. Defaults to the latest available semver tag.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
}

// Execute produces a unified diff between two refs of the component's Git source.
func (t *VendorDiffTool) Execute(_ context.Context, params map[string]interface{}) (*tools.Result, error) {
	component, err := extractRequiredStringParam(params, paramComponent)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}
	from, _ := params["from"].(string)
	to, _ := params["to"].(string)

	resolved, err := vendoring.ResolveComponentSource(&vendoring.ResolveSourceParams{Component: component})
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}
	if from == "" {
		from = resolved.Source.Version
	}

	diff, err := vendoring.Diff(nil, &vendoring.DiffParams{Source: resolved.Source.Source, From: from, To: to})
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	toDisplay := to
	if toDisplay == "" {
		toDisplay = "latest"
	}

	output := diff
	if output == "" {
		output = fmt.Sprintf("No differences between %s and %s for component %s.", from, toDisplay, component)
	}

	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			paramComponent: component,
			"from":         from,
			"to":           toDisplay,
			"diff":         diff,
		},
	}, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *VendorDiffTool) RequiresPermission() bool {
	return false // Read-only operation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *VendorDiffTool) IsRestricted() bool {
	return false
}
