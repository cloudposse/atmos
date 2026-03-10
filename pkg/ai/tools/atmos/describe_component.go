package atmos

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"

	errUtils "github.com/cloudposse/atmos/errors"
)

const (
	// Parameter name for the component.
	paramComponent = "component"
	// Parameter name for the stack.
	paramStack = "stack"
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
			Name:        paramComponent,
			Description: "Component name (e.g., 'vpc', 'rds')",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        paramStack,
			Description: "Stack name (e.g., 'prod-use1')",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
	}
}

// Execute runs the tool.
func (t *DescribeComponentTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	// Extract parameters.
	component, ok := params[paramComponent].(string)
	if !ok || component == "" {
		return &tools.Result{
			Success: false,
			Error:   errUtils.ErrAIToolParameterRequired,
		}, fmt.Errorf("%w: %s", errUtils.ErrAIToolParameterRequired, paramComponent)
	}

	stack, ok := params[paramStack].(string)
	if !ok || stack == "" {
		return &tools.Result{
			Success: false,
			Error:   errUtils.ErrAIToolParameterRequired,
		}, fmt.Errorf("%w: %s", errUtils.ErrAIToolParameterRequired, paramStack)
	}

	// Execute describe component.
	output, err := exec.ExecuteDescribeComponent(&exec.ExecuteDescribeComponentParams{
		Component:            component,
		Stack:                stack,
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		Skip:                 nil,
		AuthManager:          nil,
	})
	if err != nil {
		return &tools.Result{
			Success: false,
			Output:  "",
			Error:   err,
		}, err
	}

	return buildDescribeComponentResult(component, stack, output)
}

// buildDescribeComponentResult formats the describe component output as a tools.Result.
func buildDescribeComponentResult(component, stack string, output map[string]any) (*tools.Result, error) {
	// Convert output to YAML for better readability.
	yamlBytes, err := yaml.Marshal(output)
	if err != nil {
		// Fallback to basic string representation if YAML marshaling fails.
		outputStr := fmt.Sprintf("Component: %s\nStack: %s\n\nConfiguration:\n%+v", component, stack, output)
		//nolint:nilerr // Graceful fallback: YAML marshal error doesn't fail the tool.
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

	outputStr := fmt.Sprintf("Component: %s\nStack: %s\n\nConfiguration:\n%s", component, stack, string(yamlBytes))

	return &tools.Result{
		Success: true,
		Output:  outputStr,
		Data: map[string]interface{}{
			paramComponent: component,
			paramStack:     stack,
			"config":       output,
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
