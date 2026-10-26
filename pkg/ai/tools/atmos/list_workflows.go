package atmos

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/list/extract"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ListWorkflowsTool lists Atmos workflows with their file, description, and step count.
type ListWorkflowsTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewListWorkflowsTool creates a new list workflows tool.
func NewListWorkflowsTool(atmosConfig *schema.AtmosConfiguration) *ListWorkflowsTool {
	return &ListWorkflowsTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *ListWorkflowsTool) Name() string {
	return "atmos_list_workflows"
}

// Description returns the tool description.
func (t *ListWorkflowsTool) Description() string {
	return "List all Atmos workflows with their defining file, description, and step count. " +
		"Use this whenever you need to know what workflows exist before running or authoring one."
}

// Parameters returns the tool parameters.
func (t *ListWorkflowsTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "file",
			Description: "Restrict the listing to workflows defined in this specific workflow manifest file. Default is to scan all workflow files.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
}

// Execute runs the tool.
func (t *ListWorkflowsTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	file := ""
	if v, ok := params["file"].(string); ok {
		file = v
	}

	atmosConfig, err := currentStackConfig(t.atmosConfig)
	if err != nil {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("failed to list workflows: %w", err),
		}, err
	}

	// Execute list workflows.
	workflows, err := extract.Workflows(atmosConfig, file)
	if err != nil {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("failed to list workflows: %w", err),
		}, err
	}

	return buildListWorkflowsResult(file, workflows), nil
}

// buildListWorkflowsResult formats the workflow listing into a tools.Result.
func buildListWorkflowsResult(file string, workflows []map[string]any) *tools.Result {
	data := map[string]interface{}{
		"file":      file,
		"workflows": workflows,
		"count":     len(workflows),
	}

	// Convert output to YAML for better readability.
	yamlBytes, err := yaml.Marshal(workflows)
	if err != nil {
		// Fallback to basic string representation if YAML marshaling fails.
		outputStr := fmt.Sprintf("Workflows (%d found):\n%+v", len(workflows), workflows)
		return &tools.Result{
			Success: true,
			Output:  outputStr,
			Data:    data,
		}
	}

	outputStr := fmt.Sprintf("Workflows (%d found):\n\n%s", len(workflows), string(yamlBytes))

	return &tools.Result{
		Success: true,
		Output:  outputStr,
		Data:    data,
	}
}

// RequiresPermission returns true if this tool needs permission.
func (t *ListWorkflowsTool) RequiresPermission() bool {
	return false // Read-only operation, safe to execute.
}

// IsRestricted returns true if this tool is always restricted.
func (t *ListWorkflowsTool) IsRestricted() bool {
	return false
}
