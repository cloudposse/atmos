package atmos

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

// ToolchainRemoveTool removes a tool dependency from the project's .tool-versions file.
type ToolchainRemoveTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewToolchainRemoveTool creates a new toolchain remove tool.
func NewToolchainRemoveTool(atmosConfig *schema.AtmosConfiguration) *ToolchainRemoveTool {
	return &ToolchainRemoveTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *ToolchainRemoveTool) Name() string {
	return "atmos_toolchain_remove"
}

// Description returns the tool description.
func (t *ToolchainRemoveTool) Description() string {
	return "Remove a tool dependency from the project's .tool-versions file. When version is omitted, removes " +
		"every configured version of the tool. Requires user confirmation."
}

// Parameters returns the tool parameters.
func (t *ToolchainRemoveTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        paramTool,
			Description: "Tool name as it appears in .tool-versions (e.g. 'terraform').",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        paramVersion,
			Description: "Specific version to remove. Omit to remove all versions of the tool.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
}

// Execute removes the tool/version from .tool-versions.
func (t *ToolchainRemoveTool) Execute(_ context.Context, params map[string]interface{}) (*tools.Result, error) {
	tool, err := extractRequiredStringParam(params, paramTool)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}
	version, _ := params[paramVersion].(string)

	filePath := toolchain.GetToolVersionsFilePath()
	if err := toolchain.RemoveToolVersion(filePath, tool, version); err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	removed := tool
	if version != "" {
		removed = fmt.Sprintf("%s@%s", tool, version)
	}
	output := fmt.Sprintf("Removed %s from %s", removed, filePath)

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
func (t *ToolchainRemoveTool) RequiresPermission() bool {
	return true // Writing .tool-versions requires confirmation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *ToolchainRemoveTool) IsRestricted() bool {
	return false // User can allow via configuration.
}
