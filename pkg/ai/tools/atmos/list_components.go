package atmos

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/list/extract"
	"github.com/cloudposse/atmos/pkg/schema"
)

// paramType is the "type" tool parameter/data key shared across this file.
const paramType = "type"

// ListComponentsTool lists unique Atmos components across stacks.
type ListComponentsTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewListComponentsTool creates a new list components tool.
func NewListComponentsTool(atmosConfig *schema.AtmosConfiguration) *ListComponentsTool {
	return &ListComponentsTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *ListComponentsTool) Name() string {
	return "atmos_list_components"
}

// Description returns the tool description.
func (t *ListComponentsTool) Description() string {
	return "List unique Atmos components defined across all stacks (deduplicated by component name and type), " +
		"with the number of stacks each one appears in. Use this to discover what components exist before " +
		"describing or vendoring one."
}

// Parameters returns the tool parameters.
func (t *ListComponentsTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        paramStack,
			Description: "Glob pattern to restrict which stacks are scanned for components (e.g., 'prod-*'). Default is all stacks.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        paramType,
			Description: "Restrict results to a single component type: terraform, helmfile, packer, ansible, container, emulator, or 'all'. Default is all types.",
			Type:        tools.ParamTypeString,
			Required:    false,
			Default:     "all",
		},
		{
			Name:        "include_abstract",
			Description: "Include abstract (non-deployable) components in the results. Default is false.",
			Type:        tools.ParamTypeBool,
			Required:    false,
			Default:     false,
		},
	}
}

// Execute runs the tool.
func (t *ListComponentsTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	stack := ""
	if v, ok := params[paramStack].(string); ok {
		stack = v
	}

	componentType := ""
	if v, ok := params[paramType].(string); ok {
		componentType = v
	}

	includeAbstract := false
	if v, ok := params["include_abstract"].(bool); ok {
		includeAbstract = v
	}

	atmosConfig, err := currentStackConfig(t.atmosConfig)
	if err != nil {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("failed to list components: %w", err),
		}, err
	}

	// Describe all stacks (no auth manager: read-only introspection doesn't need per-component credentials).
	stacksMap, err := exec.ExecuteDescribeStacksWithAuthDisabled(
		atmosConfig,
		"",    // filterByStack - empty means all stacks
		nil,   // components - nil means all components
		nil,   // componentTypes - nil means all types
		nil,   // sections - nil means all sections
		false, // ignoreMissingFiles
		false, // processTemplates
		false, // processYamlFunctions
		false, // includeEmptyStacks
		nil,   // skip
		nil,   // authManager
		true,  // authDisabled
	)
	if err != nil {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("failed to list components: %w", err),
		}, err
	}

	// Extract unique components (deduplicated across all stacks).
	components, err := extract.UniqueComponents(stacksMap, stack)
	if err != nil {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("failed to list components: %w", err),
		}, err
	}

	filtered := filterListComponents(components, componentType, includeAbstract)

	return buildListComponentsResult(stack, componentType, includeAbstract, filtered), nil
}

// filterListComponents applies type and abstract filters to the unique component list.
func filterListComponents(components []map[string]any, componentType string, includeAbstract bool) []map[string]any {
	filtered := make([]map[string]any, 0, len(components))
	for _, c := range components {
		if !includeAbstract {
			if ct, ok := c["component_type"].(string); ok && ct == "abstract" {
				continue
			}
		}
		if componentType != "" && componentType != "all" {
			if ty, ok := c[paramType].(string); !ok || ty != componentType {
				continue
			}
		}
		filtered = append(filtered, c)
	}
	return filtered
}

// buildListComponentsResult formats the component listing into a tools.Result.
func buildListComponentsResult(stack, componentType string, includeAbstract bool, components []map[string]any) *tools.Result {
	data := map[string]interface{}{
		paramStack:         stack,
		paramType:          componentType,
		"include_abstract": includeAbstract,
		"components":       components,
		"count":            len(components),
	}

	// Convert output to YAML for better readability.
	yamlBytes, err := yaml.Marshal(components)
	if err != nil {
		// Fallback to basic string representation if YAML marshaling fails.
		outputStr := fmt.Sprintf("Components (%d found):\n%+v", len(components), components)
		return &tools.Result{
			Success: true,
			Output:  outputStr,
			Data:    data,
		}
	}

	outputStr := fmt.Sprintf("Components (%d found):\n\n%s", len(components), string(yamlBytes))

	return &tools.Result{
		Success: true,
		Output:  outputStr,
		Data:    data,
	}
}

// RequiresPermission returns true if this tool needs permission.
func (t *ListComponentsTool) RequiresPermission() bool {
	return false // Read-only operation, safe to execute.
}

// IsRestricted returns true if this tool is always restricted.
func (t *ListComponentsTool) IsRestricted() bool {
	return false
}
