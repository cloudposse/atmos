package engine

import (
	"encoding/json"
	"strings"
	"text/template"
	"text/template/parse"

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
//
// Go's text/template can only render text, not the typed []string a
// function like collectKeys returns, so the pipeline gets "| toJson"
// appended internally and the result is decoded with json.Unmarshal --
// transparent to scaffold.yaml authors, who never write toJson themselves.
func (p *Processor) RenderMatrixAxisExpression(expr string, answers map[string]interface{}, delimiters []string) ([]string, error) {
	defer perf.Track(nil, "engine.Processor.RenderMatrixAxisExpression")()

	delimiters = defaultAxisDelimiters(delimiters)

	funcs := buildTemplateFuncMap(answers)
	funcs["answers"] = func() map[string]interface{} { return answers }

	pipe, err := singleValuePipe(expr, delimiters, funcs)
	if err != nil {
		return nil, err
	}

	wrapped := delimiters[0] + " " + pipe.String() + " | toJson " + delimiters[1]

	tmpl, err := template.New("matrix-axis").Delims(delimiters[0], delimiters[1]).Funcs(funcs).Parse(wrapped)
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

	return parseAxisExpressionResult(result.String(), expr)
}

// singleValuePipe parses expr and returns its single pipeline, so "|
// toJson" has exactly one action to attach to: no surrounding text, no
// if/range/with, and no variable assignment (which would print nothing).
func singleValuePipe(expr string, delimiters []string, funcs template.FuncMap) (*parse.PipeNode, error) {
	tmpl, err := template.New("matrix-axis-validate").Delims(delimiters[0], delimiters[1]).Funcs(funcs).Parse(expr)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrScaffoldMatrixExpressionFailed).
			WithCause(err).
			WithExplanationf("Failed to parse matrix axis expression: `%s`", expr).
			WithHint("Check template syntax in the matrix axis value").
			Err()
	}

	nodes := tmpl.Root.Nodes
	if len(nodes) != 1 {
		return nil, errUtils.Build(errUtils.ErrScaffoldMatrixExpressionFailed).
			WithExplanationf("Matrix axis expression must be a single template action: `%s`", expr).
			WithHint("Use exactly one {{ ... }} action with no surrounding text").
			Err()
	}

	action, ok := nodes[0].(*parse.ActionNode)
	if !ok {
		return nil, errUtils.Build(errUtils.ErrScaffoldMatrixExpressionFailed).
			WithExplanationf("Matrix axis expression must be a single value-producing action: `%s`", expr).
			WithHint("if/range/with blocks aren't supported; use a single function call that returns a list").
			Err()
	}

	if action.Pipe.IsAssign || len(action.Pipe.Decl) > 0 {
		return nil, errUtils.Build(errUtils.ErrScaffoldMatrixExpressionFailed).
			WithExplanationf("Matrix axis expression must not declare or assign a variable: `%s`", expr).
			WithHint("Remove the := assignment; the expression itself must produce the list").
			Err()
	}

	return action.Pipe, nil
}

// parseAxisExpressionResult decodes a matrix axis expression's rendered
// output (always JSON -- see RenderMatrixAxisExpression) into its resolved
// list of string values.
func parseAxisExpressionResult(rendered, expr string) ([]string, error) {
	var result []string
	if err := json.Unmarshal([]byte(rendered), &result); err != nil {
		return nil, errUtils.Build(errUtils.ErrScaffoldMatrixExpressionFailed).
			WithCause(err).
			WithExplanationf("Matrix axis expression did not resolve to a list of strings: `%s`", expr).
			WithHint("Ensure the expression's function returns a list of strings").
			Err()
	}
	return result, nil
}
