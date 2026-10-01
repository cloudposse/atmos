package atmos

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
)

// DescribeDependentsTool lists Atmos components that depend on a given component.
type DescribeDependentsTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewDescribeDependentsTool creates a new describe dependents tool.
func NewDescribeDependentsTool(atmosConfig *schema.AtmosConfiguration) *DescribeDependentsTool {
	return &DescribeDependentsTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *DescribeDependentsTool) Name() string {
	return "atmos_describe_dependents"
}

// Description returns the tool description.
func (t *DescribeDependentsTool) Description() string {
	return "List the Atmos components (in any stack) that depend on the given component/stack pair. " +
		"Use this for blast-radius or change-impact analysis before modifying a component: it shows " +
		"which other components will be affected if this one changes."
}

// Parameters returns the tool parameters.
func (t *DescribeDependentsTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        paramComponent,
			Description: "Component name to find dependents for (e.g., 'vpc')",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        paramStack,
			Description: "Stack the component belongs to (e.g., 'prod-use1')",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        "include_settings",
			Description: "Include the `settings` section of each dependent component in the output. Default is false.",
			Type:        tools.ParamTypeBool,
			Required:    false,
			Default:     false,
		},
	}
}

// Execute runs the tool.
func (t *DescribeDependentsTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
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

	includeSettings := false
	if v, ok := params["include_settings"].(bool); ok {
		includeSettings = v
	}

	atmosConfig, err := currentStackConfig(t.atmosConfig)
	if err != nil {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("failed to describe dependents: %w", err),
		}, err
	}

	// Execute describe dependents.
	dependents, err := exec.ExecuteDescribeDependents(atmosConfig, &exec.DescribeDependentsArgs{
		Component:            component,
		Stack:                stack,
		IncludeSettings:      includeSettings,
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		OnlyInStack:          "", // empty string means process all stacks
	})
	if err != nil {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("failed to describe dependents: %w", err),
		}, err
	}

	return buildDescribeDependentsResult(component, stack, dependents), nil
}

// buildDescribeDependentsResult formats the dependents listing into a tools.Result.
func buildDescribeDependentsResult(component, stack string, dependents []schema.Dependent) *tools.Result {
	data := map[string]interface{}{
		paramComponent: component,
		paramStack:     stack,
		"dependents":   dependents,
		"count":        len(dependents),
	}

	// Convert output to YAML for better readability.
	yamlBytes, err := yaml.Marshal(dependents)
	if err != nil {
		// Fallback to basic string representation if YAML marshaling fails.
		outputStr := fmt.Sprintf("Dependents of '%s' in stack '%s' (%d found):\n%+v", component, stack, len(dependents), dependents)
		return &tools.Result{
			Success: true,
			Output:  outputStr,
			Data:    data,
		}
	}

	outputStr := fmt.Sprintf("Dependents of '%s' in stack '%s' (%d found):\n\n%s", component, stack, len(dependents), string(yamlBytes))

	return &tools.Result{
		Success: true,
		Output:  outputStr,
		Data:    data,
	}
}

// RequiresPermission returns true if this tool needs permission.
func (t *DescribeDependentsTool) RequiresPermission() bool {
	return false // Read-only operation, safe to execute.
}

// IsRestricted returns true if this tool is always restricted.
func (t *DescribeDependentsTool) IsRestricted() bool {
	return false
}
