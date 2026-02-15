package atmos

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ListStacksTool lists available Atmos stacks.
type ListStacksTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewListStacksTool creates a new list stacks tool.
func NewListStacksTool(atmosConfig *schema.AtmosConfiguration) *ListStacksTool {
	return &ListStacksTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *ListStacksTool) Name() string {
	return "atmos_list_stacks"
}

// Description returns the tool description.
func (t *ListStacksTool) Description() string {
	return "List all available Atmos stacks with their components. Use this tool whenever you need to know what stacks exist or what components are in each stack."
}

// Parameters returns the tool parameters.
func (t *ListStacksTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "format",
			Description: "Output format (yaml or json)",
			Type:        tools.ParamTypeString,
			Required:    false,
			Default:     "yaml",
		},
	}
}

// Execute runs the tool.
func (t *ListStacksTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	// Extract format parameter.
	format := "yaml"
	if f, ok := params["format"].(string); ok && f != "" {
		format = f
	}

	// Execute list stacks.
	// ExecuteDescribeStacks(atmosConfig, filterByStack, components, componentTypes, sections,
	//                       ignoreMissingFiles, processTemplates, processYamlFunctions, includeEmptyStacks, skip, authManager)
	stacks, err := exec.ExecuteDescribeStacks(
		t.atmosConfig,
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
	)
	if err != nil {
		return &tools.Result{
			Success: false,
			Error:   err,
		}, err
	}

	// Extract stack names and their components.
	var output strings.Builder
	output.WriteString(fmt.Sprintf("Available Stacks (%d):\n\n", len(stacks)))

	stackNames := make([]string, 0, len(stacks))
	for stackName := range stacks {
		stackNames = append(stackNames, stackName)
	}
	sort.Strings(stackNames)

	stackComponents := make(map[string][]string, len(stacks))
	for _, stackName := range stackNames {
		output.WriteString(fmt.Sprintf("Stack: %s\n", stackName))

		// Extract component names from the stack data.
		components := extractComponentNames(stacks[stackName])
		stackComponents[stackName] = components
		if len(components) > 0 {
			output.WriteString(fmt.Sprintf("  Components: %s\n", strings.Join(components, ", ")))
		} else {
			output.WriteString("  Components: (none)\n")
		}
		output.WriteString("\n")
	}

	return &tools.Result{
		Success: true,
		Output:  output.String(),
		Data: map[string]interface{}{
			"format":     format,
			"stacks":     stackNames,
			"components": stackComponents,
		},
	}, nil
}

// extractComponentNames extracts component names from a stack's data structure.
// The stack data is a map with a "components" key containing component types (e.g., "terraform")
// each mapping to individual component configurations.
func extractComponentNames(stackData any) []string {
	stackMap, ok := stackData.(map[string]any)
	if !ok {
		return nil
	}

	componentsData, ok := stackMap["components"]
	if !ok {
		return nil
	}

	componentsMap, ok := componentsData.(map[string]any)
	if !ok {
		return nil
	}

	var names []string
	// Iterate over component types (e.g., "terraform", "helmfile").
	for _, typeComponents := range componentsMap {
		typeMap, ok := typeComponents.(map[string]any)
		if !ok {
			continue
		}
		for componentName := range typeMap {
			names = append(names, componentName)
		}
	}

	sort.Strings(names)
	return names
}

// RequiresPermission returns true if this tool needs permission.
func (t *ListStacksTool) RequiresPermission() bool {
	return false // Read-only operation
}

// IsRestricted returns true if this tool is always restricted.
func (t *ListStacksTool) IsRestricted() bool {
	return false
}
