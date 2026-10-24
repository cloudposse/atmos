package atmos

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// ConfigSetTool sets a value in atmos.yaml by dot-notation path.
type ConfigSetTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewConfigSetTool creates a new atmos config set tool.
func NewConfigSetTool(atmosConfig *schema.AtmosConfiguration) *ConfigSetTool {
	return &ConfigSetTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *ConfigSetTool) Name() string {
	return "atmos_config_set"
}

// Description returns the tool description.
func (t *ConfigSetTool) Description() string {
	return "Set a value in the active atmos.yaml using a dot-notation path, preserving comments, anchors, " +
		"YAML functions, and templates. The value's type is inferred from the Atmos config schema when the " +
		"path matches a known field; pass 'type' explicitly to override. Requires user confirmation."
}

// Parameters returns the tool parameters.
func (t *ConfigSetTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "path",
			Description: "Dot-notation path to set (e.g. mcp.enabled, logs.level)",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        "value",
			Description: "The value to set, as a string (e.g. 'true', '5', 'debug')",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name: "type",
			Description: "Value type: string, int, bool, float, null, or yaml (raw literal). " +
				"Auto-inferred from the Atmos config schema when omitted and the path is recognized; falls back to string.",
			Type:     tools.ParamTypeString,
			Required: false,
		},
		{
			Name:        "file",
			Description: "Optional path to a specific atmos.yaml file to edit (overrides auto-discovery; mirrors --config)",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
}

// Execute sets the value at path in the resolved atmos.yaml.
func (t *ConfigSetTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	path, err := requireStringParam(params, "path")
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	value, err := requireStringParam(params, "value")
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	file, err := resolveConfigTargetFile(t.atmosConfig, params)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	valueType := resolveConfigSetValueType(params, path)

	created, err := atmosyaml.SetFileWithType(file, path, value, valueType)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	return buildConfigSetResult(file, path, value, valueType, created), nil
}

// resolveConfigSetValueType mirrors cmd/config/operations.go's effectiveValueType:
// an explicit "type" param wins; otherwise the value type is inferred from the
// Atmos config schema, falling back to atmosyaml.TypeString when the path isn't
// modeled by the schema (e.g. a free-form section like vars).
func resolveConfigSetValueType(params map[string]interface{}, path string) string {
	if explicit := optionalStringParam(params, "type"); explicit != "" {
		return explicit
	}
	if inferred, ok := cfg.InferValueType(path); ok {
		return inferred
	}
	return atmosyaml.TypeString
}

// buildConfigSetResult formats the outcome of a set operation into a tools.Result.
func buildConfigSetResult(file, path, value, valueType string, created bool) *tools.Result {
	displayFile := atmosyaml.DisplayPath(file)

	var output string
	if created {
		output = fmt.Sprintf("Created %s = %s (type: %s) in %s", path, value, valueType, displayFile)
	} else {
		output = fmt.Sprintf("Updated %s to %s (type: %s) in %s", path, value, valueType, displayFile)
	}

	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"path":    path,
			"value":   value,
			"type":    valueType,
			"file":    displayFile,
			"created": created,
		},
	}
}

// RequiresPermission returns true if this tool needs permission.
func (t *ConfigSetTool) RequiresPermission() bool {
	return true // Writing atmos.yaml requires confirmation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *ConfigSetTool) IsRestricted() bool {
	return false // User can allow via configuration.
}
