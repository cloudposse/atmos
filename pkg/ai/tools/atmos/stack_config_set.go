package atmos

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// paramValue is the "value" tool parameter/data key used throughout this file.
const paramValue = "value"

// StackConfigSetTool sets a component-relative value in the manifest that
// defines it for a component in a stack, using provenance to find the
// manifest file that defines the effective value.
type StackConfigSetTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewStackConfigSetTool creates a new stack config set tool.
func NewStackConfigSetTool(atmosConfig *schema.AtmosConfiguration) *StackConfigSetTool {
	return &StackConfigSetTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *StackConfigSetTool) Name() string {
	return "atmos_stack_config_set"
}

// Description returns the tool description.
func (t *StackConfigSetTool) Description() string {
	return "Set a component-relative value (e.g. vars.region) for a component in a stack. Uses provenance to find the manifest file that defines the effective value and edits that file in place, preserving comments, anchors, YAML functions, and templates. Requires user confirmation."
}

// Parameters returns the tool parameters.
func (t *StackConfigSetTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        paramStack,
			Description: "Stack name (e.g., 'plat-ue2-prod')",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        paramComponent,
			Description: "Component name (e.g., 'vpc')",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        "path",
			Description: "Component-relative dot-path to set (e.g., 'vars.region')",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        paramValue,
			Description: "Value to set at path",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        "type",
			Description: "Value type: string, int, bool, float, null, or yaml (raw literal)",
			Type:        tools.ParamTypeString,
			Required:    false,
			Default:     atmosyaml.TypeString,
		},
		{
			Name:        "file",
			Description: "Edit this manifest file explicitly instead of resolving via provenance",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
}

// Execute runs the tool.
func (t *StackConfigSetTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	p, err := extractStackConfigParams(params)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	value, ok := params[paramValue].(string)
	if !ok {
		err := fmt.Errorf("%w: %s", errUtils.ErrAIToolParameterRequired, paramValue)
		return &tools.Result{Success: false, Error: err}, err
	}

	valueType := atmosyaml.TypeString
	if vt, ok := params["type"].(string); ok && vt != "" {
		valueType = vt
	}

	atmosConfig, err := currentStackConfig(t.atmosConfig)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	tgt, err := resolveStackEditTarget(&stackEditRequest{
		atmosConfig: atmosConfig, stack: p.stack, component: p.component, dotPath: p.path, fileOverride: p.file, requireEditable: true,
	})
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	created, err := atmosyaml.SetFileWithType(tgt.file, tgt.yqPath, value, valueType)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	verb := "Updated"
	if created {
		verb = "Created"
	}
	output := fmt.Sprintf("%s `%s` = `%s` for `%s` in `%s`", verb, p.path, value, p.component, atmosyaml.DisplayPath(tgt.file))

	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			paramStack:     p.stack,
			paramComponent: p.component,
			"path":         p.path,
			paramValue:     value,
			"type":         valueType,
			"file":         tgt.file,
			"created":      created,
		},
	}, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *StackConfigSetTool) RequiresPermission() bool {
	return true // Writing stack config requires confirmation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *StackConfigSetTool) IsRestricted() bool {
	return false // User can allow via configuration.
}
