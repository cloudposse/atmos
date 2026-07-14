package atmos

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

// paramVersion is the tool version parameter shared by the toolchain add/set/remove tools.
const paramVersion = "version"

// defaultToolVersion is used when a version isn't specified for atmos_toolchain_add,
// matching `atmos toolchain add`'s own default.
const defaultToolVersion = "latest"

// ToolchainAddTool adds a tool dependency to the project's .tool-versions file.
type ToolchainAddTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewToolchainAddTool creates a new toolchain add tool.
func NewToolchainAddTool(atmosConfig *schema.AtmosConfiguration) *ToolchainAddTool {
	return &ToolchainAddTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *ToolchainAddTool) Name() string {
	return "atmos_toolchain_add"
}

// Description returns the tool description.
func (t *ToolchainAddTool) Description() string {
	return "Add a tool dependency to the project's .tool-versions file, verifying it resolves in the toolchain " +
		"registry first. If the tool is already declared, the new version is added alongside its existing " +
		"versions rather than replacing them. Requires user confirmation."
}

// Parameters returns the tool parameters.
func (t *ToolchainAddTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        paramTool,
			Description: "Tool name or owner/repo (e.g. 'terraform', 'hashicorp/terraform').",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        paramVersion,
			Description: "Version to add (e.g. '1.11.4').",
			Type:        tools.ParamTypeString,
			Required:    false,
			Default:     defaultToolVersion,
		},
	}
}

// Execute adds the tool/version to .tool-versions.
func (t *ToolchainAddTool) Execute(_ context.Context, params map[string]interface{}) (*tools.Result, error) {
	tool, err := extractRequiredStringParam(params, paramTool)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	version, _ := params[paramVersion].(string)
	if version == "" {
		version = defaultToolVersion
	}

	if err := toolchain.AddToolVersion(tool, version); err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	filePath := toolchain.GetToolVersionsFilePath()
	output := fmt.Sprintf("Added %s %s to %s", tool, version, filePath)
	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			paramTool:            tool,
			paramVersion:         version,
			"tool_versions_file": filePath,
		},
	}, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *ToolchainAddTool) RequiresPermission() bool {
	return true // Writing .tool-versions requires confirmation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *ToolchainAddTool) IsRestricted() bool {
	return false // User can allow via configuration.
}
