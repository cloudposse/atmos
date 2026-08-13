package engine

import (
	"strings"
	"text/template"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// RenderMatrixAxisExpression renders a Go-template matrix-axis expression
// (e.g. "{{ collectKeys answers.environments }}") against answers and returns
// its resolved list of string values. Satisfies the AxisRenderer type
// ExpandMatrix accepts, giving matrix axis expressions the same Gomplate,
// Sprig, and custom (including collectKeys) FuncMap scaffold templates
// already use. Delimiters is the scaffold's active left/right template
// delimiter pair (an invalid/empty value defaults to "{{"/"}}"), so a matrix
// axis expression honors a scaffold's own spec.delimiters override exactly
// like target: and file content rendering already do via
// ProcessTemplateWithDelimiters.
//
// Answers is exposed as a zero-arg function, not a data field: Go's template
// grammar only chains ".field" access off of "." or a "$var", never off a
// bare identifier -- but it does chain off a bare identifier's function-call
// result (e.g. "answers.environments" parses as "call answers(), then select
// .environments from its result"), which is what lets the expression read
// unprefixed "answers.<field>", matching how when: CEL conditions already
// expose answers.<field>, since an axis expression is a computed derivation
// from answers, not file content (which instead sees .Config.<field>).
func (p *Processor) RenderMatrixAxisExpression(expr string, answers map[string]interface{}, delimiters []string) ([]string, error) {
	defer perf.Track(nil, "engine.Processor.RenderMatrixAxisExpression")()

	delimiters = defaultAxisDelimiters(delimiters)

	funcs := buildTemplateFuncMap(answers)
	funcs["answers"] = func() map[string]interface{} { return answers }

	tmpl, err := template.New("matrix-axis").Delims(delimiters[0], delimiters[1]).Funcs(funcs).Parse(expr)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrScaffoldMatrixExpressionFailed).
			WithCause(err).
			WithExplanationf("Failed to parse matrix axis expression: `%s`", expr).
			WithHint("Check template syntax in the matrix axis value").
			Err()
	}

	var result strings.Builder
	if err := tmpl.Execute(&result, nil); err != nil {
		return nil, errUtils.Build(errUtils.ErrScaffoldMatrixExpressionFailed).
			WithCause(err).
			WithExplanationf("Failed to render matrix axis expression: `%s`", expr).
			WithHint("Check that all referenced answers exist").
			Err()
	}

	return parseAxisExpressionResult(result.String()), nil
}

// parseAxisExpressionResult parses a rendered axis expression's text output
// into a list of values. Tolerant of Go's default %v slice formatting ("[a b
// c]") as well as a plain whitespace-separated list without brackets. This
// can't distinguish "one value containing whitespace" from "two values" --
// an accepted limitation for computed axis values; see the PRD.
func parseAxisExpressionResult(rendered string) []string {
	trimmed := strings.TrimSpace(rendered)
	trimmed = strings.TrimPrefix(trimmed, "[")
	trimmed = strings.TrimSuffix(trimmed, "]")
	if trimmed == "" {
		return []string{}
	}
	return strings.Fields(trimmed)
}
