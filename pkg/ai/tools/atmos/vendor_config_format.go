package atmos

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// VendorConfigFormatTool formats a vendor manifest file in place.
type VendorConfigFormatTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewVendorConfigFormatTool creates a new vendor config format tool.
func NewVendorConfigFormatTool(atmosConfig *schema.AtmosConfiguration) *VendorConfigFormatTool {
	return &VendorConfigFormatTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *VendorConfigFormatTool) Name() string {
	return "atmos_vendor_config_format"
}

// Description returns the tool description.
func (t *VendorConfigFormatTool) Description() string {
	return "Format a vendor manifest file (vendor.yaml) in place, preserving comments, anchors, YAML functions, and templates. Only formats the resolved target file, not its imports. Requires user confirmation."
}

// Parameters returns the tool parameters.
func (t *VendorConfigFormatTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "file",
			Description: "Vendor manifest file (default: ./vendor.yaml)",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
}

// Execute runs the tool.
func (t *VendorConfigFormatTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	fileParam, _ := params["file"].(string)
	file, err := resolveVendorConfigFile(fileParam)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	if err := atmosyaml.FormatFile(file); err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	output := fmt.Sprintf("Formatted `%s`", file)

	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"file": file,
		},
	}, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *VendorConfigFormatTool) RequiresPermission() bool {
	return true // Rewriting files requires confirmation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *VendorConfigFormatTool) IsRestricted() bool {
	return false // User can allow via configuration.
}
