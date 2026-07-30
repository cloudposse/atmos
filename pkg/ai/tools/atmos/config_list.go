package atmos

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// ConfigListTool lists editable atmos.yaml setting paths.
type ConfigListTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewConfigListTool creates a new atmos config list tool.
func NewConfigListTool(atmosConfig *schema.AtmosConfiguration) *ConfigListTool {
	return &ConfigListTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *ConfigListTool) Name() string {
	return "atmos_config_list"
}

// Description returns the tool description.
func (t *ConfigListTool) Description() string {
	return "List the dot-notation setting paths defined in the active atmos.yaml, optionally filtered by a " +
		"glob-style pattern (e.g. 'toolchain.*'). Use this tool to discover what settings currently exist."
}

// Parameters returns the tool parameters.
func (t *ConfigListTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "pattern",
			Description: "Optional glob-style pattern to filter paths (e.g. 'toolchain.*'); matches all paths when omitted",
			Type:        tools.ParamTypeString,
			Required:    false,
			Default:     "",
		},
		{
			Name:        "file",
			Description: "Optional path to a specific atmos.yaml file to list (overrides auto-discovery; mirrors --config)",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
}

// Execute lists the setting paths in the resolved atmos.yaml, optionally filtered by pattern.
func (t *ConfigListTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	pattern := optionalStringParam(params, "pattern")

	file, err := resolveConfigTargetFile(t.atmosConfig, params)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	// ListPathEntries operates on raw bytes, so the file must be read explicitly.
	content, err := os.ReadFile(file)
	if err != nil {
		wrapped := fmt.Errorf("%w: %w", atmosyaml.ErrReadFile, err)
		return &tools.Result{Success: false, Error: wrapped}, wrapped
	}

	entries, err := atmosyaml.ListPathEntries(content)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	filtered := filterConfigPathEntries(entries, pattern)

	return buildConfigListResult(file, pattern, filtered), nil
}

// buildConfigListResult formats the filtered path entries into a tools.Result.
func buildConfigListResult(file, pattern string, entries []atmosyaml.PathEntry) *tools.Result {
	displayFile := atmosyaml.DisplayPath(file)

	var output strings.Builder
	fmt.Fprintf(&output, "Config paths in %s (%d):\n\n", displayFile, len(entries))

	entryMaps := make([]map[string]interface{}, 0, len(entries))
	for _, entry := range entries {
		fmt.Fprintf(&output, "%s (%s) = %s\n", entry.Path, entry.Type, entry.Value)
		entryMaps = append(entryMaps, map[string]interface{}{
			"path":  entry.Path,
			"type":  entry.Type,
			"value": entry.Value,
		})
	}

	return &tools.Result{
		Success: true,
		Output:  output.String(),
		Data: map[string]interface{}{
			"file":    displayFile,
			"pattern": pattern,
			"entries": entryMaps,
		},
	}
}

// RequiresPermission returns true if this tool needs permission.
func (t *ConfigListTool) RequiresPermission() bool {
	return false // Read-only operation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *ConfigListTool) IsRestricted() bool {
	return false
}
