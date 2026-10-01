package atmos

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	hcl "github.com/cloudposse/atmos/pkg/hcl"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TerraformComponentHCLGetTool reads an attribute or block from a Terraform
// component file by HCL address.
type TerraformComponentHCLGetTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewTerraformComponentHCLGetTool creates a new Terraform component HCL reader tool.
func NewTerraformComponentHCLGetTool(atmosConfig *schema.AtmosConfiguration) *TerraformComponentHCLGetTool {
	return &TerraformComponentHCLGetTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *TerraformComponentHCLGetTool) Name() string {
	return "atmos_terraform_component_hcl_get"
}

// Description returns the tool description.
func (t *TerraformComponentHCLGetTool) Description() string {
	return "Read an attribute value or block from a Terraform component file using an hcledit-style HCL address " +
		"(e.g. 'resource.aws_instance.web.instance_type', 'variable.region.default', or a bare block address " +
		"like 'resource.aws_instance.web'). Tries an attribute lookup first, then falls back to a block lookup. Read-only."
}

// Parameters returns the tool parameters.
func (t *TerraformComponentHCLGetTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "file_path",
			Description: "Relative path to the .tf file within the Terraform components directory (e.g. 'vpc/main.tf').",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name: "address",
			Description: "hcledit address of the attribute or block to read " +
				"(e.g. 'resource.aws_instance.web.instance_type', 'variable.region.default', 'resource.aws_instance.web').",
			Type:     tools.ParamTypeString,
			Required: true,
		},
		{
			Name:        "with_comments",
			Description: "Include the attribute's inline trailing comment in the returned value. Ignored for block results.",
			Type:        tools.ParamTypeBool,
			Required:    false,
			Default:     false,
		},
	}
}

// Execute reads the value at address from the given Terraform component file.
func (t *TerraformComponentHCLGetTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	filePath, err := extractRequiredStringParam(params, "file_path")
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	address, err := extractRequiredStringParam(params, "address")
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	withComments, _ := params["with_comments"].(bool)

	log.Debugf("Reading Terraform component HCL: %s (%s)", filePath, address)

	cleanPath, err := resolveTerraformComponentFilePath(t.atmosConfig, filePath)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	content, err := readAndValidateFile(cleanPath, filePath)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	value, err := hcl.Get(content, address, withComments)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	output := fmt.Sprintf("%s (%s):\n\n%s", filePath, address, value)
	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"file_path":     filePath,
			"address":       address,
			"value":         value,
			"with_comments": withComments,
		},
	}, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *TerraformComponentHCLGetTool) RequiresPermission() bool {
	return false // Read-only operation, safe to execute.
}

// IsRestricted returns true if this tool is always restricted.
func (t *TerraformComponentHCLGetTool) IsRestricted() bool {
	return false
}
