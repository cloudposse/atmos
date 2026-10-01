package atmos

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
)

// StackConfigGetTool reads a component-relative value from a stack manifest,
// resolving through Atmos's merge/inheritance provenance to report which
// physical manifest file defines the effective value.
type StackConfigGetTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewStackConfigGetTool creates a new stack config get tool.
func NewStackConfigGetTool(atmosConfig *schema.AtmosConfiguration) *StackConfigGetTool {
	return &StackConfigGetTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *StackConfigGetTool) Name() string {
	return "atmos_stack_config_get"
}

// Description returns the tool description.
func (t *StackConfigGetTool) Description() string {
	return "Read the effective value of a component-relative dot-path (e.g. vars.region) for a component in a stack, and report which manifest file defines it."
}

// Parameters returns the tool parameters.
func (t *StackConfigGetTool) Parameters() []tools.Parameter {
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
			Description: "Component-relative dot-path to read (e.g., 'vars.region')",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        "file",
			Description: "Read this manifest file explicitly instead of resolving via provenance",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
}

// Execute runs the tool.
func (t *StackConfigGetTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	p, err := extractStackConfigParams(params)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	atmosConfig, err := currentStackConfig(t.atmosConfig)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	tgt, err := resolveStackEditTarget(&stackEditRequest{
		atmosConfig: atmosConfig, stack: p.stack, component: p.component, dotPath: p.path, fileOverride: p.file,
	})
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	output := fmt.Sprintf("%s = %s", p.path, tgt.value)
	if tgt.provFile != "" {
		output = fmt.Sprintf("%s (resolves from %s:%d)", output, tgt.provFile, tgt.provLine)
	}

	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			paramStack:     p.stack,
			paramComponent: p.component,
			"path":         p.path,
			"value":        tgt.value,
			"file":         tgt.provFile,
			"line":         tgt.provLine,
		},
	}, nil
}

// stackConfigParams holds the common stack/component/path parameters shared
// by the stack config tools, plus the optional file override.
type stackConfigParams struct {
	stack     string
	component string
	path      string
	file      string
}

// extractStackConfigParams extracts and validates the common stack/component/path
// parameters shared by the stack config tools, plus the optional file override.
func extractStackConfigParams(params map[string]interface{}) (stackConfigParams, error) {
	stack, ok := params[paramStack].(string)
	if !ok || stack == "" {
		return stackConfigParams{}, fmt.Errorf("%w: %s", errUtils.ErrAIToolParameterRequired, paramStack)
	}

	component, ok := params[paramComponent].(string)
	if !ok || component == "" {
		return stackConfigParams{}, fmt.Errorf("%w: %s", errUtils.ErrAIToolParameterRequired, paramComponent)
	}

	path, ok := params["path"].(string)
	if !ok || path == "" {
		return stackConfigParams{}, fmt.Errorf("%w: %s", errUtils.ErrAIToolParameterRequired, "path")
	}

	p := stackConfigParams{stack: stack, component: component, path: path}
	if f, ok := params["file"].(string); ok {
		p.file = f
	}

	return p, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *StackConfigGetTool) RequiresPermission() bool {
	return false // Read-only operation, safe to execute.
}

// IsRestricted returns true if this tool is always restricted.
func (t *StackConfigGetTool) IsRestricted() bool {
	return false
}
