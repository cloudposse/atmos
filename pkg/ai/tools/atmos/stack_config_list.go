package atmos

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// StackConfigListTool enumerates the addressable component-relative dot-paths
// in a component's merged stack configuration, along with each path's type,
// effective value, and the manifest file that defines it (via provenance).
type StackConfigListTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewStackConfigListTool creates a new stack config list tool.
func NewStackConfigListTool(atmosConfig *schema.AtmosConfiguration) *StackConfigListTool {
	return &StackConfigListTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *StackConfigListTool) Name() string {
	return "atmos_stack_config_list"
}

// Description returns the tool description.
func (t *StackConfigListTool) Description() string {
	return "List editable component-relative config paths (e.g. vars.region) for a component in a stack, along with each path's type, effective value, and the manifest file that defines it."
}

// Parameters returns the tool parameters.
func (t *StackConfigListTool) Parameters() []tools.Parameter {
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
			Name:        "pattern",
			Description: "Optional glob pattern to filter paths (e.g., 'vars.*')",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
}

// stackConfigPathRow is one row in the stack config list result.
type stackConfigPathRow struct {
	Path  string
	Type  string
	Value string
	File  string
}

// Execute runs the tool.
func (t *StackConfigListTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	stack, ok := params[paramStack].(string)
	if !ok || stack == "" {
		err := fmt.Errorf("%w: %s", errUtils.ErrAIToolParameterRequired, paramStack)
		return &tools.Result{Success: false, Error: err}, err
	}

	component, ok := params[paramComponent].(string)
	if !ok || component == "" {
		err := fmt.Errorf("%w: %s", errUtils.ErrAIToolParameterRequired, paramComponent)
		return &tools.Result{Success: false, Error: err}, err
	}

	pattern := ""
	if p, ok := params["pattern"].(string); ok {
		pattern = p
	}

	atmosConfig, err := currentStackConfig(t.atmosConfig)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	result, err := describeStackComponentForEdit(atmosConfig, stack, component)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	rows, err := buildStackConfigListRows(atmosConfig, result, component, pattern)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	return buildStackConfigListResult(stack, component, pattern, rows), nil
}

// buildStackConfigListRows flattens the component's merged section into
// addressable dot-paths, maps each back to its provenance file, and applies
// an optional glob pattern filter. Mirrors cmd/stack/config.go's
// buildStackConfigRowsFromDescribe.
func buildStackConfigListRows(atmosConfig *schema.AtmosConfiguration, result *exec.DescribeComponentResult, component, pattern string) ([]stackConfigPathRow, error) {
	sectionYAML, err := u.ConvertToYAML(result.ComponentSection)
	if err != nil {
		return nil, err
	}
	entries, err := atmosyaml.ListPathEntries([]byte(sectionYAML))
	if err != nil {
		return nil, err
	}

	var patternRe *regexp.Regexp
	if pattern != "" {
		patternRe = stackConfigListPatternRegexp(pattern)
	}

	componentType, _ := result.ComponentSection[cfg.ComponentTypeSectionName].(string)
	rows := make([]stackConfigPathRow, 0, len(entries))
	for _, entry := range entries {
		if patternRe != nil && !patternRe.MatchString(entry.Path) {
			continue
		}
		file, ok := stackProvenanceFileForPath(atmosConfig, result, componentType, component, entry.Path)
		if !ok {
			continue
		}
		rows = append(rows, stackConfigPathRow{
			Path:  entry.Path,
			Type:  entry.Type,
			Value: entry.Value,
			File:  file,
		})
	}
	return rows, nil
}

// stackConfigListPatternRegexp converts a simple glob pattern (wildcards *
// and any single character) into an anchored regular expression, matching
// pkg/list's path-pattern glob semantics used by `atmos stack config list`.
func stackConfigListPatternRegexp(pattern string) *regexp.Regexp {
	quoted := regexp.QuoteMeta(pattern)
	quoted = strings.ReplaceAll(quoted, `\*`, `.*`)
	quoted = strings.ReplaceAll(quoted, `\?`, `.`)
	return regexp.MustCompile("^" + quoted + "$")
}

// buildStackConfigListResult formats the path rows into a tools.Result.
func buildStackConfigListResult(stack, component, pattern string, rows []stackConfigPathRow) *tools.Result {
	var output strings.Builder
	fmt.Fprintf(&output, "Config paths for `%s` in `%s` (%d):\n\n", component, stack, len(rows))

	entries := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		fmt.Fprintf(&output, "%s = %s (%s) [%s]\n", row.Path, row.Value, row.Type, row.File)
		entries = append(entries, map[string]interface{}{
			"path":  row.Path,
			"type":  row.Type,
			"value": row.Value,
			"file":  row.File,
		})
	}

	return &tools.Result{
		Success: true,
		Output:  output.String(),
		Data: map[string]interface{}{
			paramStack:     stack,
			paramComponent: component,
			"pattern":      pattern,
			"entries":      entries,
		},
	}
}

// RequiresPermission returns true if this tool needs permission.
func (t *StackConfigListTool) RequiresPermission() bool {
	return false // Read-only operation, safe to execute.
}

// IsRestricted returns true if this tool is always restricted.
func (t *StackConfigListTool) IsRestricted() bool {
	return false
}
