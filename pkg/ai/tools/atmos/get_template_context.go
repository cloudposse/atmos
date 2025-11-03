package atmos

import (
	"context"
	"encoding/json"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// GetTemplateContextTool gets the template context for debugging Go template failures.
type GetTemplateContextTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewGetTemplateContextTool creates a new template context tool.
func NewGetTemplateContextTool(atmosConfig *schema.AtmosConfiguration) *GetTemplateContextTool {
	return &GetTemplateContextTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *GetTemplateContextTool) Name() string {
	return "get_template_context"
}

// Description returns the tool description.
func (t *GetTemplateContextTool) Description() string {
	return "Get the template context (variables and functions) available for a component in a stack. Use this to debug Go template failures, see what atmos.Component() returns, and understand available template functions. Returns the full context that can be used in Go templates."
}

// Parameters returns the tool parameters.
func (t *GetTemplateContextTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "component",
			Description: "The component name (e.g., 'vpc', 'eks', 'rds')",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        "stack",
			Description: "The stack name (e.g., 'prod-us-east-1', 'staging-eu-west-1')",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
	}
}

// Execute retrieves the template context for the component.
func (t *GetTemplateContextTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	// Extract component parameter.
	component, ok := params["component"].(string)
	if !ok || component == "" {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: component", errUtils.ErrAIToolParameterRequired),
		}, nil
	}

	// Extract stack parameter.
	stack, ok := params["stack"].(string)
	if !ok || stack == "" {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: stack", errUtils.ErrAIToolParameterRequired),
		}, nil
	}

	log.Debug(fmt.Sprintf("Getting template context for component '%s' in stack '%s'", component, stack))

	// Get component configuration which contains the template context.
	componentConfig, err := e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
		Component:            component,
		Stack:                stack,
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Skip:                 []string{},
		AuthManager:          nil,
	})
	if err != nil {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("failed to get component configuration: %w", err),
		}, nil
	}

	// Format the context as JSON for readability.
	contextJSON, err := json.MarshalIndent(componentConfig, "", "  ")
	if err != nil {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("failed to format context: %w", err),
		}, nil
	}

	output := fmt.Sprintf("Template context for component '%s' in stack '%s':\n\n%s\n\n", component, stack, string(contextJSON))
	output += "Available template functions:\n"
	output += "- atmos.Component(component, stack) - Get component configuration\n"
	output += "- atmos.Stack(stack) - Get stack configuration\n"
	output += "- atmos.Setting(key) - Get Atmos setting\n"
	output += "- terraform.output(component, stack, output_name) - Get Terraform output\n"
	output += "- terraform.state(component, stack) - Get Terraform state\n"
	output += "- store.get(key) - Get value from configured store (SSM, Azure Key Vault, etc.)\n"
	output += "- exec(command) - Execute shell command\n"
	output += "- env(var_name) - Get environment variable\n"
	output += "Plus all Gomplate functions: https://docs.gomplate.ca/functions/"

	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"component": component,
			"stack":     stack,
			"context":   componentConfig,
		},
	}, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *GetTemplateContextTool) RequiresPermission() bool {
	return false // Read-only operation, safe to execute.
}

// IsRestricted returns true if this tool is always restricted.
func (t *GetTemplateContextTool) IsRestricted() bool {
	return false
}
