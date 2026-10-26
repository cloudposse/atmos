package atmos

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// VendorConfigSetTool sets a raw value in a vendor manifest by dot-notation path.
type VendorConfigSetTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewVendorConfigSetTool creates a new vendor config set tool.
func NewVendorConfigSetTool(atmosConfig *schema.AtmosConfiguration) *VendorConfigSetTool {
	return &VendorConfigSetTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *VendorConfigSetTool) Name() string {
	return "atmos_vendor_config_set"
}

// Description returns the tool description.
func (t *VendorConfigSetTool) Description() string {
	return "Set a raw value in a vendor manifest (vendor.yaml) by dot-notation path (e.g. 'spec.sources[0].version'). Requires user confirmation."
}

// Parameters returns the tool parameters.
func (t *VendorConfigSetTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "path",
			Description: "Dot-notation path to set (e.g. 'spec.sources[0].version')",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        "value",
			Description: "Value to write at path",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        "type",
			Description: "Value type: string, int, bool, float, null, or yaml (raw literal). Defaults to string.",
			Type:        tools.ParamTypeString,
			Required:    false,
			Default:     atmosyaml.TypeString,
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
func (t *VendorConfigSetTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		err := fmt.Errorf("%w: path", errUtils.ErrAIToolParameterRequired)
		return &tools.Result{Success: false, Error: err}, err
	}

	value, ok := params["value"].(string)
	if !ok {
		err := fmt.Errorf("%w: value", errUtils.ErrAIToolParameterRequired)
		return &tools.Result{Success: false, Error: err}, err
	}

	valueType := atmosyaml.TypeString
	if vt, ok := params["type"].(string); ok && vt != "" {
		valueType = vt
	}

	fileParam, _ := params["file"].(string)
	file, err := resolveVendorConfigFile(fileParam)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	created, err := atmosyaml.SetFileWithType(file, path, value, valueType)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	action := "Updated"
	if created {
		action = "Created"
	}
	output := fmt.Sprintf("%s `%s` = `%s` in `%s`", action, path, value, file)

	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"file":    file,
			"path":    path,
			"value":   value,
			"type":    valueType,
			"created": created,
		},
	}, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *VendorConfigSetTool) RequiresPermission() bool {
	return true // Writing files requires confirmation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *VendorConfigSetTool) IsRestricted() bool {
	return false // User can allow via configuration.
}
