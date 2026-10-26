package atmos

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// Parameter name for the path to the JSON Schema or OPA policy file used for validation.
	paramSchemaPath = "schema_path"
	// Parameter name for the validation schema type (jsonschema or opa).
	paramSchemaType = "schema_type"
	// Parameter name for the OPA policy module/catalog paths.
	paramModulePaths = "module_paths"
	// Parameter name for the validation timeout in seconds.
	paramTimeout = "timeout"
)

// ValidateComponentTool validates a single Atmos component in a stack using JSON Schema or OPA policies.
type ValidateComponentTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewValidateComponentTool creates a new validate component tool.
func NewValidateComponentTool(atmosConfig *schema.AtmosConfiguration) *ValidateComponentTool {
	return &ValidateComponentTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *ValidateComponentTool) Name() string {
	return "atmos_validate_component"
}

// Description returns the tool description.
func (t *ValidateComponentTool) Description() string {
	return "Validate an Atmos component's configuration in a specific stack using JSON Schema or OPA " +
		"policies. Read-only; if no schema is given, uses the component's configured `settings.validation`."
}

// Parameters returns the tool parameters.
func (t *ValidateComponentTool) Parameters() []tools.Parameter {
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
		{
			Name:        paramSchemaPath,
			Description: "Path to the JSON Schema or OPA policy file. When omitted, uses the component's configured `settings.validation`.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        paramSchemaType,
			Description: "Schema type: jsonschema or opa. Required when schema_path is set.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        paramModulePaths,
			Description: "Paths to OPA policy modules or catalogs (only used with schema_type=opa).",
			Type:        tools.ParamTypeArray,
			Required:    false,
		},
		{
			Name:        paramTimeout,
			Description: "Validation timeout in seconds.",
			Type:        tools.ParamTypeInt,
			Required:    false,
			Default:     0,
		},
	}
}

// Execute runs the tool.
func (t *ValidateComponentTool) Execute(_ context.Context, params map[string]interface{}) (*tools.Result, error) {
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

	schemaPath, _ := params[paramSchemaPath].(string)
	schemaType, _ := params[paramSchemaType].(string)
	modulePaths := extractModulePathsParam(params)
	timeoutSeconds := extractTimeoutParam(params)

	atmosConfig, err := currentStackConfig(t.atmosConfig)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	ok2, err := exec.ExecuteValidateComponent(
		atmosConfig,
		schema.ConfigAndStacksInfo{},
		component,
		stack,
		schemaPath,
		schemaType,
		modulePaths,
		timeoutSeconds,
	)
	if err != nil {
		return &tools.Result{
			Success: false,
			Output:  fmt.Sprintf("Validation failed: %v", err),
			Error:   err,
		}, err
	}

	return buildValidateComponentResult(component, stack, ok2), nil
}

// buildValidateComponentResult formats the validation outcome into a tools.Result.
func buildValidateComponentResult(component, stack string, valid bool) *tools.Result {
	status := "failed"
	icon := "❌" // X mark.
	if valid {
		status = "passed"
		icon = "✅" // Check mark.
	}

	output := fmt.Sprintf("%s Component %q in stack %q validation %s\n", icon, component, stack, status)

	return &tools.Result{
		Success: valid,
		Output:  output,
		Data: map[string]interface{}{
			paramComponent: component,
			paramStack:     stack,
			"valid":        valid,
		},
	}
}

// extractModulePathsParam extracts the module_paths parameter, which arrives as []interface{}
// (the shape produced when tool arguments are decoded from JSON).
func extractModulePathsParam(params map[string]interface{}) []string {
	raw, ok := params[paramModulePaths]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// extractTimeoutParam extracts the timeout parameter, which may arrive as float64 (JSON), int, or int64.
func extractTimeoutParam(params map[string]interface{}) int {
	raw, ok := params[paramTimeout]
	if !ok || raw == nil {
		return 0
	}
	switch v := raw.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

// RequiresPermission returns true if this tool needs permission.
func (t *ValidateComponentTool) RequiresPermission() bool {
	return false // Read-only validation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *ValidateComponentTool) IsRestricted() bool {
	return false
}
