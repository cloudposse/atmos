package atmos

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ValidateSchemaTool validates YAML files against the JSON Schemas configured
// in the `schemas` section of atmos.yaml. With no key, this includes the
// built-in `config` entry, which validates atmos.yaml itself (plus atmos.d
// fragments and project-local profiles) against the schema generated from the
// Atmos configuration code.
type ValidateSchemaTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewValidateSchemaTool creates a new validate schema tool.
func NewValidateSchemaTool(atmosConfig *schema.AtmosConfiguration) *ValidateSchemaTool {
	return &ValidateSchemaTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *ValidateSchemaTool) Name() string {
	return "atmos_validate_schema"
}

// Description returns the tool description.
func (t *ValidateSchemaTool) Description() string {
	return "Validate YAML files against the JSON Schemas configured in atmos.yaml, including atmos.yaml itself (with atmos.d and profile fragments) via the built-in `config` schema entry"
}

// Parameters returns the tool parameters.
func (t *ValidateSchemaTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "key",
			Description: "Schema registry key to validate (e.g. `config` for atmos.yaml only); empty validates every configured entry",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
}

// Execute runs the tool.
func (t *ValidateSchemaTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	key := ""
	if k, ok := params["key"].(string); ok {
		key = k
	}

	if err := exec.NewAtmosValidatorExecutor(t.atmosConfig).ExecuteAtmosValidateSchemaCmd(key, ""); err != nil {
		return &tools.Result{
			Success: false,
			Output:  fmt.Sprintf("Schema validation failed: %v", err),
			Error:   err,
		}, err
	}

	return &tools.Result{
		Success: true,
		Output:  "✅ All schemas validated successfully",
		Data: map[string]interface{}{
			"key": key,
		},
	}, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *ValidateSchemaTool) RequiresPermission() bool {
	return false // Read-only validation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *ValidateSchemaTool) IsRestricted() bool {
	return false
}
