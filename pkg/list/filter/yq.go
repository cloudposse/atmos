package filter

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
)

// Row keys preserved across projection so default columns still resolve.
const (
	keyStack     = "stack"
	keyComponent = "component"
)

// YQPredicateFilter keeps rows where the YQ expression evaluates to a truthy
// value. The expression is evaluated once per row against the row's own map
// (so users write paths like `.vars.region`, not `.<stack>.vars.region`).
//
// Truthy semantics mirror YAML/JSON intuition rather than yq's internal
// boolean operators: `true` is truthy; `false`, `nil`, and empty
// strings/maps/slices are falsy; non-empty scalars/maps/slices are truthy.
//
// Errors evaluating the expression against a single row are propagated up so
// the user sees an invalid-expression diagnostic instead of a silent empty
// result — yq syntax errors surface on the first row.
type YQPredicateFilter struct {
	Expr        string
	AtmosConfig *schema.AtmosConfiguration
}

// NewYQPredicateFilter creates a filter that keeps rows for which Expr is truthy.
// Returns an error if Expr is empty.
func NewYQPredicateFilter(expr string, atmosConfig *schema.AtmosConfiguration) (*YQPredicateFilter, error) {
	if expr == "" {
		return nil, fmt.Errorf("%w: yq filter expression cannot be empty", errUtils.ErrInvalidConfig)
	}
	return &YQPredicateFilter{Expr: expr, AtmosConfig: atmosConfig}, nil
}

// Apply evaluates the predicate against each row and returns the matching subset.
func (f *YQPredicateFilter) Apply(data interface{}) (interface{}, error) {
	defer perf.Track(nil, "filter.YQPredicateFilter.Apply")()

	items, ok := data.([]map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: expected []map[string]any, got %T", errUtils.ErrInvalidConfig, data)
	}

	var filtered []map[string]any
	for _, item := range items {
		result, err := utils.EvaluateYqExpression(f.AtmosConfig, item, f.Expr)
		if err != nil {
			return nil, fmt.Errorf("%w: filter expression %q failed: %w", errUtils.ErrInvalidConfig, f.Expr, err)
		}
		if isTruthy(result) {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

// YQProjector replaces each row with the YQ expression result. Used to wire
// `--query` on commands like `atmos list instances`.
//
// When the expression returns a map, that map becomes the new row (its keys
// are addressable as column templates). When it returns a scalar, the row is
// rewritten to a single "value" column. When the expression returns nil for a
// row, that row is dropped.
type YQProjector struct {
	Expr        string
	AtmosConfig *schema.AtmosConfiguration
}

// NewYQProjector creates a projector that rewrites each row using Expr.
// Returns an error if Expr is empty.
func NewYQProjector(expr string, atmosConfig *schema.AtmosConfiguration) (*YQProjector, error) {
	if expr == "" {
		return nil, fmt.Errorf("%w: yq projection expression cannot be empty", errUtils.ErrInvalidConfig)
	}
	return &YQProjector{Expr: expr, AtmosConfig: atmosConfig}, nil
}

// Apply runs the projector over each row of data.
func (p *YQProjector) Apply(data interface{}) (interface{}, error) {
	defer perf.Track(nil, "filter.YQProjector.Apply")()

	items, ok := data.([]map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: expected []map[string]any, got %T", errUtils.ErrInvalidConfig, data)
	}

	projected := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result, err := utils.EvaluateYqExpression(p.AtmosConfig, item, p.Expr)
		if err != nil {
			return nil, fmt.Errorf("%w: query expression %q failed: %w", errUtils.ErrInvalidConfig, p.Expr, err)
		}
		if result == nil {
			continue
		}
		row := projectionRow(item, result)
		if row == nil {
			continue
		}
		projected = append(projected, row)
	}
	return projected, nil
}

// projectionRow builds the new row for one input row given the YQ result.
// Map results overlay onto a stack/component skeleton so the default columns
// (Stack, Component) still resolve when present in the original row; scalar
// results land in a "value" column.
func projectionRow(original map[string]any, result any) map[string]any {
	switch v := result.(type) {
	case map[string]any:
		if len(v) == 0 {
			return nil
		}
		row := make(map[string]any, len(v)+2)
		// Preserve stack/component identity for column resolution.
		if s, ok := original[keyStack]; ok {
			row[keyStack] = s
		}
		if c, ok := original[keyComponent]; ok {
			row[keyComponent] = c
		}
		for k, val := range v {
			row[k] = val
		}
		return row
	default:
		row := map[string]any{"value": v}
		if s, ok := original[keyStack]; ok {
			row[keyStack] = s
		}
		if c, ok := original[keyComponent]; ok {
			row[keyComponent] = c
		}
		return row
	}
}

// isTruthy reports whether a YQ result should be treated as a passing
// predicate. Matches the intuition that bare-path expressions act like
// presence checks while comparison expressions return concrete booleans.
func isTruthy(v any) bool {
	switch val := v.(type) {
	case nil:
		return false
	case bool:
		return val
	case string:
		return isTruthyString(val)
	case int:
		return val != 0
	case int64:
		return val != 0
	case float64:
		return val != 0
	case []any:
		return len(val) > 0
	case map[string]any:
		return len(val) > 0
	default:
		return true
	}
}

// isTruthyString returns false for empty / "false" / "null" string literals,
// which look like predicate failures to a human reading the output.
func isTruthyString(s string) bool {
	if s == "" || s == "false" || s == "null" {
		return false
	}
	return true
}
