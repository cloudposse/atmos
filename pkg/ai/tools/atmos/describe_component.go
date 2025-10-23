package atmos

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"

	errUtils "github.com/cloudposse/atmos/errors"
)

// DescribeComponentTool describes an Atmos component in a stack.
type DescribeComponentTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewDescribeComponentTool creates a new describe component tool.
func NewDescribeComponentTool(atmosConfig *schema.AtmosConfiguration) *DescribeComponentTool {
	return &DescribeComponentTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *DescribeComponentTool) Name() string {
	return "atmos_describe_component"
}

// Description returns the tool description.
func (t *DescribeComponentTool) Description() string {
	return "Describe an Atmos component configuration in a specific stack"
}

// Parameters returns the tool parameters.
func (t *DescribeComponentTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "component",
			Description: "Component name (e.g., 'vpc', 'rds')",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        "stack",
			Description: "Stack name (e.g., 'prod-use1')",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
	}
}

// Execute runs the tool.
func (t *DescribeComponentTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	// Extract parameters.
	component, ok := params["component"].(string)
	if !ok || component == "" {
		return &tools.Result{
			Success: false,
			Error:   errUtils.ErrAIToolParameterRequired,
		}, fmt.Errorf("%w: component", errUtils.ErrAIToolParameterRequired)
	}

	stack, ok := params["stack"].(string)
	if !ok || stack == "" {
		return &tools.Result{
			Success: false,
			Error:   errUtils.ErrAIToolParameterRequired,
		}, fmt.Errorf("%w: stack", errUtils.ErrAIToolParameterRequired)
	}

	// Execute describe component.
	// ExecuteDescribeComponent(component, stack, processTemplates, processYamlFunctions, skip)
	output, err := exec.ExecuteDescribeComponent(component, stack, false, false, nil)
	if err != nil {
		return &tools.Result{
			Success: false,
			Output:  "",
			Error:   err,
		}, err
	}

	// Convert output to formatted string.
	outputStr := fmt.Sprintf("Component: %s\nStack: %s\n\nConfiguration:\n%+v", component, stack, output)

	return &tools.Result{
		Success: true,
		Output:  outputStr,
		Data: map[string]interface{}{
			"component": component,
			"stack":     stack,
			"config":    output,
		},
	}, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *DescribeComponentTool) RequiresPermission() bool {
	return false // Read-only operation, safe to execute
}

// IsRestricted returns true if this tool is always restricted.
func (t *DescribeComponentTool) IsRestricted() bool {
	return false
}
