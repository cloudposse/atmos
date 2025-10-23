package atmos

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ValidateStacksTool validates Atmos stack configurations.
type ValidateStacksTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewValidateStacksTool creates a new validate stacks tool.
func NewValidateStacksTool(atmosConfig *schema.AtmosConfiguration) *ValidateStacksTool {
	return &ValidateStacksTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *ValidateStacksTool) Name() string {
	return "atmos_validate_stacks"
}

// Description returns the tool description.
func (t *ValidateStacksTool) Description() string {
	return "Validate Atmos stack configurations for correctness"
}

// Parameters returns the tool parameters.
func (t *ValidateStacksTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "schema_type",
			Description: "Schema type to validate against (jsonschema, opa)",
			Type:        tools.ParamTypeString,
			Required:    false,
			Default:     "jsonschema",
		},
	}
}

// Execute runs the tool.
func (t *ValidateStacksTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	// Extract schema type (for future use if we add schema-specific validation).
	schemaType := "jsonschema"
	if st, ok := params["schema_type"].(string); ok && st != "" {
		schemaType = st
	}

	// Execute validation using ValidateStacks.
	// Note: ValidateStacks uses the schema configured in atmos.yaml
	err := exec.ValidateStacks(t.atmosConfig)
	if err != nil {
		return &tools.Result{
			Success: false,
			Output:  fmt.Sprintf("Validation failed: %v", err),
			Error:   err,
		}, err
	}

	return &tools.Result{
		Success: true,
		Output:  "✅ All stacks validated successfully",
		Data: map[string]interface{}{
			"schema_type": schemaType,
		},
	}, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *ValidateStacksTool) RequiresPermission() bool {
	return false // Read-only validation
}

// IsRestricted returns true if this tool is always restricted.
func (t *ValidateStacksTool) IsRestricted() bool {
	return false
}
