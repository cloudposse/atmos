package atmos

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// DescribeWorkflowsTool lists all discovered Atmos workflows and the files that define them.
type DescribeWorkflowsTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewDescribeWorkflowsTool creates a new describe workflows tool.
func NewDescribeWorkflowsTool(atmosConfig *schema.AtmosConfiguration) *DescribeWorkflowsTool {
	return &DescribeWorkflowsTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *DescribeWorkflowsTool) Name() string {
	return "atmos_describe_workflows"
}

// Description returns the tool description.
func (t *DescribeWorkflowsTool) Description() string {
	return "List all Atmos workflows discovered under the configured workflows base path, showing which " +
		"file defines each workflow. Use this to discover what automation is already available before " +
		"writing new workflows or CLI commands."
}

// Parameters returns the tool parameters.
func (t *DescribeWorkflowsTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "output_type",
			Description: "Shape of the returned data: 'list' (flat list of file/workflow pairs), 'map' (workflow names grouped by file), or 'all' (full workflow manifests). Default is 'list'.",
			Type:        tools.ParamTypeString,
			Required:    false,
			Default:     "list",
		},
		{
			Name:        "query",
			Description: "Optional YQ expression to filter/transform the result (e.g., '.[] | select(.file == \"deploy.yaml\")').",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
}

// parseDescribeWorkflowsParams applies defaults for the optional output_type/query parameters.
func parseDescribeWorkflowsParams(params map[string]interface{}) (outputType, query string) {
	outputType = "list"
	if v, ok := params["output_type"].(string); ok && v != "" {
		outputType = v
	}
	if v, ok := params["query"].(string); ok {
		query = v
	}
	return outputType, query
}

// selectDescribeWorkflowsOutput picks the raw result shape matching outputType.
func selectDescribeWorkflowsOutput(outputType string, list []schema.DescribeWorkflowsItem, byFile map[string][]string, manifests map[string]schema.WorkflowManifest) (any, error) {
	switch outputType {
	case "list":
		return list, nil
	case "map":
		return byFile, nil
	case "all":
		return manifests, nil
	default:
		return nil, fmt.Errorf("%w: %q", errUtils.ErrAIToolInvalidOutputType, outputType)
	}
}

// Execute runs the tool.
func (t *DescribeWorkflowsTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	outputType, query := parseDescribeWorkflowsParams(params)

	atmosConfig, err := currentStackConfig(t.atmosConfig)
	if err != nil {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("failed to describe workflows: %w", err),
		}, err
	}

	// Execute describe workflows.
	workflowList, workflowMap, workflowManifests, err := exec.ExecuteDescribeWorkflows(*atmosConfig)
	if err != nil {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("failed to describe workflows: %w", err),
		}, err
	}

	res, err := selectDescribeWorkflowsOutput(outputType, workflowList, workflowMap, workflowManifests)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	if query != "" {
		res, err = u.EvaluateYqExpression(atmosConfig, res, query)
		if err != nil {
			return &tools.Result{
				Success: false,
				Error:   fmt.Errorf("failed to evaluate query: %w", err),
			}, err
		}
	}

	return buildDescribeWorkflowsResult(outputType, query, workflowList, res), nil
}

// buildDescribeWorkflowsResult formats the workflows listing into a tools.Result.
func buildDescribeWorkflowsResult(outputType, query string, workflowList []schema.DescribeWorkflowsItem, res any) *tools.Result {
	data := map[string]interface{}{
		"output_type": outputType,
		"query":       query,
		"workflows":   workflowList,
		"count":       len(workflowList),
		"result":      res,
	}

	// Convert output to YAML for better readability.
	yamlBytes, err := yaml.Marshal(res)
	if err != nil {
		// Fallback to basic string representation if YAML marshaling fails.
		outputStr := fmt.Sprintf("Workflows (%d found, output_type=%s):\n%+v", len(workflowList), outputType, res)
		return &tools.Result{
			Success: true,
			Output:  outputStr,
			Data:    data,
		}
	}

	outputStr := fmt.Sprintf("Workflows (%d found, output_type=%s):\n\n%s", len(workflowList), outputType, string(yamlBytes))

	return &tools.Result{
		Success: true,
		Output:  outputStr,
		Data:    data,
	}
}

// RequiresPermission returns true if this tool needs permission.
func (t *DescribeWorkflowsTool) RequiresPermission() bool {
	return false // Read-only operation, safe to execute.
}

// IsRestricted returns true if this tool is always restricted.
func (t *DescribeWorkflowsTool) IsRestricted() bool {
	return false
}
