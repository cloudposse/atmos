package atmos

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// ConfigDeleteTool deletes a value from atmos.yaml by dot-notation path.
type ConfigDeleteTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewConfigDeleteTool creates a new atmos config delete tool.
func NewConfigDeleteTool(atmosConfig *schema.AtmosConfiguration) *ConfigDeleteTool {
	return &ConfigDeleteTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *ConfigDeleteTool) Name() string {
	return "atmos_config_delete"
}

// Description returns the tool description.
func (t *ConfigDeleteTool) Description() string {
	return "Delete a value from the active atmos.yaml using a dot-notation path, preserving the rest of the " +
		"file. Requires user confirmation."
}

// Parameters returns the tool parameters.
func (t *ConfigDeleteTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "path",
			Description: "Dot-notation path to delete (e.g. components.terraform.append_user_agent)",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        "file",
			Description: "Optional path to a specific atmos.yaml file to edit (overrides auto-discovery; mirrors --config)",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
}

// Execute deletes the value at path from the resolved atmos.yaml.
func (t *ConfigDeleteTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	path, err := requireStringParam(params, "path")
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	file, err := resolveConfigTargetFile(t.atmosConfig, params)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	existed, err := atmosyaml.DeleteFile(file, path)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	displayFile := atmosyaml.DisplayPath(file)

	var output string
	if !existed {
		output = fmt.Sprintf("Nothing to delete: %s is not set in %s", path, displayFile)
	} else {
		output = fmt.Sprintf("Deleted %s from %s", path, displayFile)
	}

	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"path":    path,
			"file":    displayFile,
			"deleted": existed,
		},
	}, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *ConfigDeleteTool) RequiresPermission() bool {
	return true // Deleting from atmos.yaml requires confirmation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *ConfigDeleteTool) IsRestricted() bool {
	return false // User can allow via configuration.
}
