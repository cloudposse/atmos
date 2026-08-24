package atmos

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// ConfigGetTool reads a value from atmos.yaml by dot-notation path.
type ConfigGetTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewConfigGetTool creates a new atmos config get tool.
func NewConfigGetTool(atmosConfig *schema.AtmosConfiguration) *ConfigGetTool {
	return &ConfigGetTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *ConfigGetTool) Name() string {
	return "atmos_config_get"
}

// Description returns the tool description.
func (t *ConfigGetTool) Description() string {
	return "Read a value from the active atmos.yaml using a dot-notation path (e.g. mcp.enabled). " +
		"Use this tool to inspect the current Atmos configuration."
}

// Parameters returns the tool parameters.
func (t *ConfigGetTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "path",
			Description: "Dot-notation path to read (e.g. mcp.enabled, logs.level)",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        "file",
			Description: "Optional path to a specific atmos.yaml file to read from (overrides auto-discovery; mirrors --config)",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
}

// Execute reads the value at path from the resolved atmos.yaml.
func (t *ConfigGetTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	path, err := requireStringParam(params, "path")
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	file, err := resolveConfigTargetFile(t.atmosConfig, params)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	value, err := atmosyaml.GetFile(file, path)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	displayFile := atmosyaml.DisplayPath(file)
	return &tools.Result{
		Success: true,
		Output:  fmt.Sprintf("%s = %s (from %s)", path, value, displayFile),
		Data: map[string]interface{}{
			"path":  path,
			"value": value,
			"file":  displayFile,
		},
	}, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *ConfigGetTool) RequiresPermission() bool {
	return false // Read-only operation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *ConfigGetTool) IsRestricted() bool {
	return false
}
