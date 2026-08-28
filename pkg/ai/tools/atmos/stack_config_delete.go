package atmos

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// StackConfigDeleteTool deletes a component-relative value from the manifest
// that defines it for a component in a stack, using provenance to find the
// manifest file that defines the effective value.
type StackConfigDeleteTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewStackConfigDeleteTool creates a new stack config delete tool.
func NewStackConfigDeleteTool(atmosConfig *schema.AtmosConfiguration) *StackConfigDeleteTool {
	return &StackConfigDeleteTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *StackConfigDeleteTool) Name() string {
	return "atmos_stack_config_delete"
}

// Description returns the tool description.
func (t *StackConfigDeleteTool) Description() string {
	return "Delete a component-relative value (e.g. settings.spacelift.workspace_enabled) from the manifest that defines it for a component in a stack. Uses provenance to find the manifest file that defines the effective value. Requires user confirmation."
}

// Parameters returns the tool parameters.
func (t *StackConfigDeleteTool) Parameters() []tools.Parameter {
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
			Description: "Component-relative dot-path to delete (e.g., 'settings.spacelift.workspace_enabled')",
			Type:        tools.ParamTypeString,
			Required:    true,
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
func (t *StackConfigDeleteTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	p, err := extractStackConfigParams(params)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
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

	existed, err := atmosyaml.DeleteFile(tgt.file, tgt.yqPath)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	output := fmt.Sprintf("Nothing to delete -- `%s` for `%s` is not set in `%s`", p.path, p.component, atmosyaml.DisplayPath(tgt.file))
	if existed {
		output = fmt.Sprintf("Deleted `%s` for `%s` from `%s`", p.path, p.component, atmosyaml.DisplayPath(tgt.file))
	}

	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			paramStack:     p.stack,
			paramComponent: p.component,
			"path":         p.path,
			"file":         tgt.file,
			"deleted":      existed,
		},
	}, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *StackConfigDeleteTool) RequiresPermission() bool {
	return true // Deleting stack config requires confirmation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *StackConfigDeleteTool) IsRestricted() bool {
	return false // User can allow via configuration.
}
