package atmos

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

// toolchainSetScrollSpeed is passed to SetToolVersion's interactive-picker UI
// parameter. It's irrelevant here since version is always required
// (non-empty), so the interactive picker path never runs; matches the CLI's
// own default when a version is supplied directly.
const toolchainSetScrollSpeed = 0

// ToolchainSetTool sets a tool's default pinned version in .tool-versions.
type ToolchainSetTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewToolchainSetTool creates a new toolchain set tool.
func NewToolchainSetTool(atmosConfig *schema.AtmosConfiguration) *ToolchainSetTool {
	return &ToolchainSetTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *ToolchainSetTool) Name() string {
	return "atmos_toolchain_set"
}

// Description returns the tool description.
func (t *ToolchainSetTool) Description() string {
	return "Set a tool's default pinned version in the project's .tool-versions file, resolving the tool to its " +
		"canonical owner/repo form. Requires user confirmation."
}

// Parameters returns the tool parameters.
func (t *ToolchainSetTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        paramTool,
			Description: "Tool name or owner/repo (e.g. 'terraform', 'hashicorp/terraform').",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name: paramVersion,
			Description: "Version to set as default (e.g. '1.11.4'). Required -- an empty version would " +
				"trigger an interactive picker in the CLI, which this tool does not support.",
			Type:     tools.ParamTypeString,
			Required: true,
		},
	}
}

// Execute sets the tool's default version in .tool-versions.
func (t *ToolchainSetTool) Execute(_ context.Context, params map[string]interface{}) (*tools.Result, error) {
	tool, err := extractRequiredStringParam(params, paramTool)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}
	version, err := extractRequiredStringParam(params, paramVersion)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	if err := toolchain.SetToolVersion(tool, version, toolchainSetScrollSpeed); err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	filePath := toolchain.GetToolVersionsFilePath()
	output := fmt.Sprintf("Set %s to %s in %s", tool, version, filePath)
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
func (t *ToolchainSetTool) RequiresPermission() bool {
	return true // Writing .tool-versions requires confirmation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *ToolchainSetTool) IsRestricted() bool {
	return false // User can allow via configuration.
}
