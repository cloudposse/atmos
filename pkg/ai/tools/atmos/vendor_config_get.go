package atmos

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// VendorConfigGetTool reads a raw value from a vendor manifest by dot-notation path.
type VendorConfigGetTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewVendorConfigGetTool creates a new vendor config get tool.
func NewVendorConfigGetTool(atmosConfig *schema.AtmosConfiguration) *VendorConfigGetTool {
	return &VendorConfigGetTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *VendorConfigGetTool) Name() string {
	return "atmos_vendor_config_get"
}

// Description returns the tool description.
func (t *VendorConfigGetTool) Description() string {
	return "Read a raw value from a vendor manifest (vendor.yaml) by dot-notation path (e.g. 'spec.sources[0].version')."
}

// Parameters returns the tool parameters.
func (t *VendorConfigGetTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "path",
			Description: "Dot-notation path to read (e.g. 'spec.sources[0].version')",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        "file",
			Description: "Vendor manifest file (default: ./vendor.yaml)",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
}

// Execute runs the tool.
func (t *VendorConfigGetTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		err := fmt.Errorf("%w: path", errUtils.ErrAIToolParameterRequired)
		return &tools.Result{Success: false, Error: err}, err
	}

	fileParam, _ := params["file"].(string)
	file, err := resolveVendorConfigFile(fileParam)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	value, err := atmosyaml.GetFile(file, path)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	return &tools.Result{
		Success: true,
		Output:  value,
		Data: map[string]interface{}{
			"file":  file,
			"path":  path,
			"value": value,
		},
	}, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *VendorConfigGetTool) RequiresPermission() bool {
	return false // Read-only operation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *VendorConfigGetTool) IsRestricted() bool {
	return false
}
