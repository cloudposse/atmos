package atmos

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// StackConfigFormatTool formats every manifest file that contributes to a
// component's effective configuration in a stack, using provenance to find
// all contributing files rather than a single one.
type StackConfigFormatTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewStackConfigFormatTool creates a new stack config format tool.
func NewStackConfigFormatTool(atmosConfig *schema.AtmosConfiguration) *StackConfigFormatTool {
	return &StackConfigFormatTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *StackConfigFormatTool) Name() string {
	return "atmos_stack_config_format"
}

// Description returns the tool description.
func (t *StackConfigFormatTool) Description() string {
	return "Format the manifest files that define a stack component in place. Uses provenance to find every manifest file that contributes effective component values and normalizes each. Requires user confirmation."
}

// Parameters returns the tool parameters.
func (t *StackConfigFormatTool) Parameters() []tools.Parameter {
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
	}
}

// Execute runs the tool.
func (t *StackConfigFormatTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	stack, ok := params[paramStack].(string)
	if !ok || stack == "" {
		err := fmt.Errorf("%w: %s", errUtils.ErrAIToolParameterRequired, paramStack)
		return &tools.Result{Success: false, Error: err}, err
	}

	component, ok := params[paramComponent].(string)
	if !ok || component == "" {
		err := fmt.Errorf("%w: %s", errUtils.ErrAIToolParameterRequired, paramComponent)
		return &tools.Result{Success: false, Error: err}, err
	}

	atmosConfig, err := currentStackConfig(t.atmosConfig)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	result, err := describeStackComponentForEdit(atmosConfig, stack, component)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	files, err := stackFormatFilesFromProvenance(atmosConfig, result, stack, component)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	for _, file := range files {
		if err := atmosyaml.FormatFile(file); err != nil {
			return &tools.Result{Success: false, Error: err}, err
		}
	}

	output := fmt.Sprintf("Formatted %d stack config file(s) for `%s` in `%s`.", len(files), component, stack)

	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			paramStack:     stack,
			paramComponent: component,
			"files":        files,
		},
	}, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *StackConfigFormatTool) RequiresPermission() bool {
	return true // Formatting rewrites files on disk, requires confirmation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *StackConfigFormatTool) IsRestricted() bool {
	return false // User can allow via configuration.
}
