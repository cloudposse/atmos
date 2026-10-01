package atmos

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// ConfigFormatTool formats the active atmos.yaml file in place.
type ConfigFormatTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewConfigFormatTool creates a new atmos config format tool.
func NewConfigFormatTool(atmosConfig *schema.AtmosConfiguration) *ConfigFormatTool {
	return &ConfigFormatTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *ConfigFormatTool) Name() string {
	return "atmos_config_format"
}

// Description returns the tool description.
func (t *ConfigFormatTool) Description() string {
	return "Format the active atmos.yaml file in place, preserving comments, anchors, YAML functions, and " +
		"templates. Requires user confirmation."
}

// Parameters returns the tool parameters.
func (t *ConfigFormatTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "file",
			Description: "Optional path to a specific atmos.yaml file to format (overrides auto-discovery; mirrors --config)",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
}

// Execute formats the resolved atmos.yaml in place.
func (t *ConfigFormatTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	file, err := resolveConfigTargetFile(t.atmosConfig, params)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	if err := atmosyaml.FormatFile(file); err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	displayFile := atmosyaml.DisplayPath(file)
	return &tools.Result{
		Success: true,
		Output:  fmt.Sprintf("Formatted %s", displayFile),
		Data: map[string]interface{}{
			"file": displayFile,
		},
	}, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *ConfigFormatTool) RequiresPermission() bool {
	return true // Rewriting atmos.yaml requires confirmation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *ConfigFormatTool) IsRestricted() bool {
	return false // User can allow via configuration.
}
