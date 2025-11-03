package atmos

import (
	"context"
	"fmt"
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
	return "List all available Atmos stacks"
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

	// Format output - extract stack names from the map.
	var output strings.Builder
	output.WriteString(fmt.Sprintf("Available Stacks (%s format):\n\n", format))

	// Extract stack names from the stacks map.
	stackNames := make([]string, 0, len(stacks))
	for stackName := range stacks {
		stackNames = append(stackNames, stackName)
	}

	// Write stack names.
	for _, name := range stackNames {
		output.WriteString(fmt.Sprintf("- %s\n", name))
	}

	return &tools.Result{
		Success: true,
		Output:  output.String(),
		Data: map[string]interface{}{
			"format": format,
			"stacks": stackNames,
		},
	}, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *ListStacksTool) RequiresPermission() bool {
	return false // Read-only operation
}

// IsRestricted returns true if this tool is always restricted.
func (t *ListStacksTool) IsRestricted() bool {
	return false
}
