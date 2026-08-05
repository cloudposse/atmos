package engine

import (
	"fmt"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// MatrixKey is the reserved answers key one resolved matrix combination
// travels under, from ExpandMatrix's result being stashed in the
// per-combination values map (see pkg/generator/ui) through to
// ProcessTemplateWithDelimiters, which hoists it onto the template root as
// "matrix" -- so a template writes .matrix.<axis> directly, matching the
// namespace the workflow matrix step's own {{ .matrix.<axis> }} uses.
const MatrixKey = "Matrix"

// answersPrefix is the required prefix for a dynamic matrix axis source, a
// root reference into the answers map -- see resolveMatrixAxis.
const answersPrefix = "answers."

// AxisRenderer renders a Go-template matrix-axis expression (e.g. "{{ keys
// answers.environments }}") against answers into its resolved list of
// string values. A nil renderer disables template-expression axis support,
// keeping ExpandMatrix's core algorithm -- and its many existing pure unit
// tests -- free of any templating engine dependency. See
// Processor.RenderMatrixAxisExpression in funcs.go for the real
// implementation.
type AxisRenderer func(expr string, answers map[string]interface{}) ([]string, error)

// ExpandMatrix resolves a FileSpec's Matrix axes against answers and returns
// their full Cartesian product as one row (map[axis]value) per combination,
// expanded in a sorted, deterministic order per axis -- the same behavior
// pkg/workflow's own matrix step expansion has, so regenerating the same
// answers produces the same file set, in the same order, every time.
func ExpandMatrix(matrix map[string]any, answers map[string]interface{}, render AxisRenderer) ([]map[string]string, error) {
	defer perf.Track(nil, "engine.ExpandMatrix")()

	resolved := make(map[string][]string, len(matrix))
	for axis, raw := range matrix {
		values, err := resolveMatrixAxis(axis, raw, answers, render)
		if err != nil {
			return nil, err
		}
		resolved[axis] = values
	}
	return cartesianProduct(resolved), nil
}

// resolveMatrixAxis resolves one axis's declared value into its list of
// string values: a literal list is used as-is; a string is either a
// Go-template expression (containing "{{", rendered via render, e.g. to
// compute a list from nested/structured answer data with keys) or a dot-path
// into answers.* (validated at load time to require that prefix) that must
// resolve to an already list-shaped value.
func resolveMatrixAxis(axis string, raw any, answers map[string]interface{}, render AxisRenderer) ([]string, error) {
	switch v := raw.(type) {
	case []string:
		return v, nil
	case []any:
		values := make([]string, len(v))
		for i, item := range v {
			values[i] = toString(item)
		}
		return values, nil
	case string:
		if strings.Contains(v, "{{") {
			return resolveMatrixAxisExpression(axis, v, answers, render)
		}
		return resolveMatrixAxisFromAnswers(axis, v, answers)
	default:
		return nil, errUtils.Build(errUtils.ErrScaffoldMatrixAxisInvalid).
			WithExplanationf("matrix axis %q resolved to %T", axis, raw).
			Err()
	}
}

// resolveMatrixAxisExpression renders a template-expression axis value via
// render. A nil render (e.g. ExpandMatrix's pure unit tests, or any caller
// that hasn't wired a Processor) is a clear error rather than a silent empty
// axis.
func resolveMatrixAxisExpression(axis, expr string, answers map[string]interface{}, render AxisRenderer) ([]string, error) {
	if render == nil {
		return nil, errUtils.Build(errUtils.ErrScaffoldMatrixExpressionFailed).
			WithExplanationf("matrix axis %q is a template expression, but no template renderer is available", axis).
			Err()
	}
	return render(expr, answers)
}

// resolveMatrixAxisFromAnswers resolves a dynamic axis source (a dot-path
// string with the "answers" prefix) against the answers map, requiring the
// resolved value to already be list-shaped -- a multiselect answer, or a
// structured value supplied through --set or a template-declared preset.
func resolveMatrixAxisFromAnswers(axis, source string, answers map[string]interface{}) ([]string, error) {
	path, ok := strings.CutPrefix(source, answersPrefix)
	if !ok {
		return nil, errUtils.Build(errUtils.ErrScaffoldMatrixAxisInvalid).
			WithExplanationf("matrix axis %q: %q does not start with %q", axis, source, answersPrefix).
			WithHint("A dynamic matrix axis must reference a top-level answer, e.g. answers.environments").
			Err()
	}

	var current interface{} = answers
	for _, segment := range strings.Split(path, ".") {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil, errUtils.Build(errUtils.ErrScaffoldMatrixSourceNotFound).
				WithExplanationf("matrix axis %q source %q: %q is not a map", axis, source, segment).
				Err()
		}
		value, exists := m[segment]
		if !exists {
			return nil, errUtils.Build(errUtils.ErrScaffoldMatrixSourceNotFound).
				WithExplanationf("matrix axis %q source %q not found in answers", axis, source).
				Err()
		}
		current = value
	}

	switch v := current.(type) {
	case nil:
		return nil, nil
	case []string:
		return v, nil
	case []any:
		values := make([]string, len(v))
		for i, item := range v {
			values[i] = toString(item)
		}
		return values, nil
	default:
		return nil, errUtils.Build(errUtils.ErrScaffoldMatrixSourceNotList).
			WithExplanationf("matrix axis %q source %q resolved to %T", axis, source, current).
			WithHint("Reference a multiselect answer, or a list-shaped value from --set or a template preset").
			Err()
	}
}

// toString renders a resolved axis value's element as a plain string,
// matching the workflow matrix step's own map[string][]string axis shape.
func toString(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprint(value)
}

// cartesianProduct expands axes into their full Cartesian product, one row
// (map[axis]value) per combination, iterating axes in sorted order so
// regenerating the same answers produces the same file set, in the same
// order, every time -- mirroring pkg/workflow/control_matrix.go's
// expandMatrix, which this intentionally parallels rather than imports (a
// pure, dependency-free algorithm; importing pkg/workflow into the generator
// package for it would pull in an unrelated subsystem for a few lines of
// logic).
func cartesianProduct(matrix map[string][]string) []map[string]string {
	axes := make([]string, 0, len(matrix))
	for axis := range matrix {
		axes = append(axes, axis)
	}
	sort.Strings(axes)

	rows := []map[string]string{{}}
	for _, axis := range axes {
		next := make([]map[string]string, 0, len(rows))
		for _, row := range rows {
			for _, value := range matrix[axis] {
				copied := make(map[string]string, len(row)+1)
				for k, v := range row {
					copied[k] = v
				}
				copied[axis] = value
				next = append(next, copied)
			}
		}
		rows = next
	}
	return rows
}
