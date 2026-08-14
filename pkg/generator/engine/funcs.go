package engine

import (
	"strings"
	"text/template"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// RenderMatrixAxisExpression renders a Go-template matrix-axis expression
// (e.g. "{{ collectKeys answers.environments }}") against answers and
// returns its resolved list of string values. Satisfies the AxisRenderer
// type ExpandMatrix accepts, so an axis expression gets the same FuncMap
// and delimiters override scaffold templates already use.
//
// Answers is exposed as a zero-arg function rather than a data field: Go's
// template grammar only chains ".field" off "." or a "$var", never off a
// bare identifier -- but it does chain off that identifier's call result,
// which is what lets "answers.environments" read as "call answers(), then
// select .environments".
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
// into a list of values, tolerating both a bracketed Go slice ("[a b c]")
// and a plain space-separated list. A value containing whitespace collapses
// into multiple values -- an accepted limitation; see the PRD.
func parseAxisExpressionResult(rendered string) []string {
	trimmed := strings.TrimSpace(rendered)
	trimmed = strings.TrimPrefix(trimmed, "[")
	trimmed = strings.TrimSuffix(trimmed, "]")
	if trimmed == "" {
		return []string{}
	}
	return strings.Fields(trimmed)
}
