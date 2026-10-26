package atmos

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// VendorConfigDeleteTool deletes a raw value from a vendor manifest by dot-notation path.
type VendorConfigDeleteTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewVendorConfigDeleteTool creates a new vendor config delete tool.
func NewVendorConfigDeleteTool(atmosConfig *schema.AtmosConfiguration) *VendorConfigDeleteTool {
	return &VendorConfigDeleteTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *VendorConfigDeleteTool) Name() string {
	return "atmos_vendor_config_delete"
}

// Description returns the tool description.
func (t *VendorConfigDeleteTool) Description() string {
	return "Delete a raw value from a vendor manifest (vendor.yaml) by dot-notation path (e.g. 'spec.sources[0].tags'). Requires user confirmation."
}

// Parameters returns the tool parameters.
func (t *VendorConfigDeleteTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "path",
			Description: "Dot-notation path to delete (e.g. 'spec.sources[0].tags')",
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
func (t *VendorConfigDeleteTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
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

	existed, err := atmosyaml.DeleteFile(file, path)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	output := fmt.Sprintf("Nothing to delete — `%s` is not set in `%s`", path, file)
	if existed {
		output = fmt.Sprintf("Deleted `%s` from `%s`", path, file)
	}

	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"file":    file,
			"path":    path,
			"deleted": existed,
		},
	}, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *VendorConfigDeleteTool) RequiresPermission() bool {
	return true // Deleting values requires confirmation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *VendorConfigDeleteTool) IsRestricted() bool {
	return false // User can allow via configuration.
}
