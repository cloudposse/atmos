package atmos

import (
	"context"
	"encoding/json"
	"fmt"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// DescribeAffectedTool shows affected components based on git changes.
type DescribeAffectedTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewDescribeAffectedTool creates a new describe affected tool.
func NewDescribeAffectedTool(atmosConfig *schema.AtmosConfiguration) *DescribeAffectedTool {
	return &DescribeAffectedTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *DescribeAffectedTool) Name() string {
	return "describe_affected"
}

// Description returns the tool description.
func (t *DescribeAffectedTool) Description() string {
	return "Show components affected by git changes. Use this for change impact analysis, understanding what components will be affected by your changes, and planning deployments. Compares the current branch with a reference (default: main branch)."
}

// Parameters returns the tool parameters.
func (t *DescribeAffectedTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "ref",
			Description: "Git reference to compare against (e.g., 'main', 'HEAD~1', 'origin/main'). Default is 'main'.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        "verbose",
			Description: "Include detailed information about affected files. Default is false.",
			Type:        tools.ParamTypeBool,
			Required:    false,
		},
	}
}

// Execute retrieves affected components.
func (t *DescribeAffectedTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	// Extract optional ref parameter.
	ref := "main"
	if r, ok := params["ref"].(string); ok && r != "" {
		ref = r
	}

	// Extract optional verbose parameter.
	verbose := false
	if v, ok := params["verbose"].(bool); ok {
		verbose = v
	}

	log.Debug(fmt.Sprintf("Describing affected components (ref: %s, verbose: %v)", ref, verbose))

	// Execute describe affected command.
	affected, _, _, _, err := e.ExecuteDescribeAffectedWithTargetRepoPath(
		t.atmosConfig,
		ref,
		false,      // includeSpaceliftAdminStacks
		false,      // includeSettings
		"",         // stack (empty for all)
		true,       // processTemplates
		true,       // processYamlFunctions
		[]string{}, // skip
		false,      // excludeLocked
	)
	if err != nil {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("failed to describe affected: %w", err),
		}, nil
	}

	// Format output.
	var output string
	if verbose {
		// Include full details.
		affectedJSON, err := json.MarshalIndent(affected, "", "  ")
		if err != nil {
			return &tools.Result{
				Success: false,
				Error:   fmt.Errorf("failed to format output: %w", err),
			}, nil
		}
		output = fmt.Sprintf("Affected components (compared to %s):\n\n%s", ref, string(affectedJSON))
	} else {
		// Summarize affected components.
		output = fmt.Sprintf("Affected components (compared to %s):\n\n", ref)

		if len(affected) == 0 {
			output += "No affected components found.\n"
		} else {
			output += fmt.Sprintf("Total: %d components\n\n", len(affected))
			for _, item := range affected {
				output += fmt.Sprintf("- %s in stack %s\n", item.Component, item.Stack)
			}
		}
	}

	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"ref":      ref,
			"verbose":  verbose,
			"affected": affected,
		},
	}, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *DescribeAffectedTool) RequiresPermission() bool {
	return false // Read-only operation, safe to execute.
}

// IsRestricted returns true if this tool is always restricted.
func (t *DescribeAffectedTool) IsRestricted() bool {
	return false
}
