package atmos

import (
	"context"
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	l "github.com/cloudposse/atmos/pkg/list"
	listerrors "github.com/cloudposse/atmos/pkg/list/errors"
	listutils "github.com/cloudposse/atmos/pkg/list/utils"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	paramQuery  = "query"
	paramFormat = "format"
)

// listValuesParams holds the parsed, defaulted parameters for ListValuesTool.Execute.
type listValuesParams struct {
	component       string
	stack           string
	query           string
	format          string
	includeAbstract bool
}

// ListValuesTool lists a component's values (or vars) across every stack where it is used.
type ListValuesTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewListValuesTool creates a new list values tool.
func NewListValuesTool(atmosConfig *schema.AtmosConfiguration) *ListValuesTool {
	return &ListValuesTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *ListValuesTool) Name() string {
	return "atmos_list_values"
}

// Description returns the tool description.
func (t *ListValuesTool) Description() string {
	return "List a component's values (or a subset selected by a YQ query, such as `.vars`) across every " +
		"stack where the component is used. Use this for cross-stack comparison, e.g. \"what is `region` " +
		"set to across every stack for the vpc component?\"."
}

// Parameters returns the tool parameters.
func (t *ListValuesTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        paramComponent,
			Description: "Component name to list values for (e.g., 'vpc')",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        paramStack,
			Description: "Glob pattern to restrict which stacks are scanned (e.g., 'prod-*'). Default is all stacks.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        paramQuery,
			Description: "YQ expression selecting the section to list (e.g., '.vars', '.settings'). Default is the full component section (vars merged with metadata).",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        "vars",
			Description: "Shortcut for query='.vars': show only the component's vars across stacks. Default is false.",
			Type:        tools.ParamTypeBool,
			Required:    false,
			Default:     false,
		},
		{
			Name:        "include_abstract",
			Description: "Include stacks where the component is defined abstractly. Default is false.",
			Type:        tools.ParamTypeBool,
			Required:    false,
			Default:     false,
		},
		{
			Name:        paramFormat,
			Description: "Output format: yaml, json, csv, or tsv. Default is 'yaml'.",
			Type:        tools.ParamTypeString,
			Required:    false,
			Default:     "yaml",
		},
	}
}

// resolveListValuesQuery returns the query parameter, or the ".vars" shortcut
// when the caller set vars=true and didn't also pass an explicit query.
func resolveListValuesQuery(params map[string]interface{}) string {
	query := ""
	if v, ok := params[paramQuery].(string); ok {
		query = v
	}
	if query != "" {
		return query
	}
	if v, ok := params["vars"].(bool); ok && v {
		return ".vars"
	}
	return query
}

// parseListValuesParams validates the required component parameter and applies
// defaults for the rest, including the vars=true shortcut for query='.vars'.
func parseListValuesParams(params map[string]interface{}) (listValuesParams, error) {
	component, ok := params[paramComponent].(string)
	if !ok || component == "" {
		return listValuesParams{}, fmt.Errorf("%w: %s", errUtils.ErrAIToolParameterRequired, paramComponent)
	}

	p := listValuesParams{component: component, format: "yaml", query: resolveListValuesQuery(params)}
	if v, ok := params[paramStack].(string); ok {
		p.stack = v
	}
	if v, ok := params["include_abstract"].(bool); ok {
		p.includeAbstract = v
	}
	if v, ok := params[paramFormat].(string); ok && v != "" {
		p.format = v
	}
	return p, nil
}

// Execute runs the tool.
func (t *ListValuesTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	p, err := parseListValuesParams(params)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	atmosConfig, err := currentStackConfig(t.atmosConfig)
	if err != nil {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("failed to list values: %w", err),
		}, err
	}

	if !listutils.CheckComponentExists(atmosConfig, p.component) {
		notFoundErr := &listerrors.ComponentDefinitionNotFoundError{Component: p.component}
		return &tools.Result{
			Success: false,
			Error:   notFoundErr,
		}, notFoundErr
	}

	// Describe all stacks (no auth manager: read-only introspection doesn't need per-component credentials).
	stacksMap, err := exec.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("failed to list values: %w", err),
		}, err
	}

	return buildListValuesResult(stacksMap, p)
}

// buildListValuesResult runs the filter/query and formats the result, including the
// "no values found" success path FilterAndListValues reports as a typed error.
func buildListValuesResult(stacksMap map[string]any, p listValuesParams) (*tools.Result, error) {
	filterOptions := &l.FilterOptions{
		Component:       p.component,
		ComponentFilter: p.component,
		Query:           p.query,
		IncludeAbstract: p.includeAbstract,
		FormatStr:       p.format,
		StackPattern:    p.stack,
	}
	if p.query == ".vars" {
		// Mirrors `atmos list vars`: clear Component so the extractor builds the vars-specific query path.
		filterOptions.Component = ""
	}

	data := map[string]interface{}{
		paramComponent: p.component,
		paramStack:     p.stack,
		paramQuery:     p.query,
		paramFormat:    p.format,
	}

	output, err := l.FilterAndListValues(stacksMap, filterOptions)
	if err != nil {
		var noValuesErr *listerrors.NoValuesFoundError
		if errors.As(err, &noValuesErr) {
			return &tools.Result{
				Success: true,
				Output:  fmt.Sprintf("No values found for component '%s' with query '%s'.", p.component, p.query),
				Data:    data,
			}, nil
		}
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("failed to list values: %w", err),
		}, err
	}

	return &tools.Result{
		Success: true,
		Output:  output,
		Data:    data,
	}, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *ListValuesTool) RequiresPermission() bool {
	return false // Read-only operation, safe to execute.
}

// IsRestricted returns true if this tool is always restricted.
func (t *ListValuesTool) IsRestricted() bool {
	return false
}
