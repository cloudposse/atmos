package engine

import (
	"encoding/json"
	"strings"
	"text/template"
	"text/template/parse"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// RenderAnswersListExpression renders a Go-template expression (e.g. "{{
// collectKeys answers.environments }}") against answers and returns its
// resolved list of string values. Satisfies the AxisRenderer type
// ExpandMatrix accepts and the FieldOptionsRenderer type a scaffold field's
// dynamic `options:` accepts (pkg/project/config duplicates that type
// rather than importing this one -- see its own doc comment for the
// import-cycle reason) -- both a matrix axis expression and an `options:`
// expression get the same FuncMap and delimiters override scaffold
// templates already use, and the same error reporting, since neither
// caller is more "correct" to name this after.
//
// A thin list-shaped specialization of RenderAnswersExpression: see that
// method's doc comment for the shared rendering mechanics (answers
// binding, the "| toJson" wrap, JSON decoding).
func (p *Processor) RenderAnswersListExpression(expr string, answers map[string]interface{}, delimiters []string) ([]string, error) {
	defer perf.Track(nil, "engine.Processor.RenderAnswersListExpression")()

	rendered, err := p.renderAnswersExpressionJSON(expr, answers, delimiters)
	if err != nil {
		return nil, err
	}

	return parseExpressionResult(rendered, expr)
}

// RenderAnswersExpression renders a Go-template expression against answers
// and returns its resolved value decoded from JSON, whatever shape the
// expression produces (string, bool, number, list, or map) -- the general
// counterpart to RenderAnswersListExpression, which requires and decodes a
// list of strings specifically. Exported for callers that need an
// answers-derived value of arbitrary shape rather than a list.
//
// Answers is exposed as a zero-arg function rather than a data field: Go's
// template grammar only chains ".field" off "." or a "$var", never off a
// bare identifier -- but it does chain off that identifier's call result,
// which is what lets "answers.environments" read as "call answers(), then
// select .environments".
//
// Go's text/template can only render text, not a typed value like the
// []string collectKeys returns or the bool/number/map a computed field's
// expression might produce, so the pipeline gets "| toJson" appended
// internally and the result is decoded with json.Unmarshal -- transparent
// to scaffold.yaml authors, who never write toJson themselves.
func (p *Processor) RenderAnswersExpression(expr string, answers map[string]interface{}, delimiters []string) (any, error) {
	defer perf.Track(nil, "engine.Processor.RenderAnswersExpression")()

	rendered, err := p.renderAnswersExpressionJSON(expr, answers, delimiters)
	if err != nil {
		return nil, err
	}

	return parseExpressionResultAny(rendered, expr)
}

// renderAnswersExpressionJSON parses expr as a single Go-template action
// (see singleValuePipe), appends "| toJson" to it, and executes it against
// answers, returning the raw JSON-encoded result string both
// RenderAnswersListExpression and RenderAnswersExpression decode -- the
// mechanics are identical regardless of the shape the caller ultimately
// wants back.
func (p *Processor) renderAnswersExpressionJSON(expr string, answers map[string]interface{}, delimiters []string) (string, error) {
	delimiters = defaultDelimiters(delimiters)

	funcs := buildTemplateFuncMap(answers)
	funcs["answers"] = func() map[string]interface{} { return answers }

	pipe, err := singleValuePipe(expr, delimiters, funcs)
	if err != nil {
		return "", err
	}

	wrapped := delimiters[0] + " " + pipe.String() + " | toJson " + delimiters[1]

	tmpl, err := template.New("answers-expression").Delims(delimiters[0], delimiters[1]).Funcs(funcs).Parse(wrapped)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrScaffoldExpressionFailed).
			WithCause(err).
			WithExplanationf("Failed to parse template expression: `%s`", expr).
			WithHint("Check template syntax in the expression").
			Err()
	}

	var result strings.Builder
	if err := tmpl.Execute(&result, nil); err != nil {
		return "", errUtils.Build(errUtils.ErrScaffoldExpressionFailed).
			WithCause(err).
			WithExplanationf("Failed to render template expression: `%s`", expr).
			WithHint("Check that all referenced answers exist").
			Err()
	}

	return result.String(), nil
}

// singleValuePipe parses expr and returns its single pipeline, so "|
// toJson" has exactly one action to attach to: no surrounding text, no
// if/range/with, and no variable assignment (which would print nothing).
// Shared by both RenderAnswersListExpression and RenderAnswersExpression --
// the single-action constraint is about the shape of the template, not the
// shape of the value it resolves to.
func singleValuePipe(expr string, delimiters []string, funcs template.FuncMap) (*parse.PipeNode, error) {
	tmpl, err := template.New("answers-expression-validate").Delims(delimiters[0], delimiters[1]).Funcs(funcs).Parse(expr)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrScaffoldExpressionFailed).
			WithCause(err).
			WithExplanationf("Failed to parse template expression: `%s`", expr).
			WithHint("Check template syntax in the expression").
			Err()
	}

	nodes := tmpl.Root.Nodes
	if len(nodes) != 1 {
		return nil, errUtils.Build(errUtils.ErrScaffoldExpressionFailed).
			WithExplanationf("Template expression must be a single template action: `%s`", expr).
			WithHint("Use exactly one " + delimiters[0] + " ... " + delimiters[1] + " action with no surrounding text").
			Err()
	}

	action, ok := nodes[0].(*parse.ActionNode)
	if !ok {
		return nil, errUtils.Build(errUtils.ErrScaffoldExpressionFailed).
			WithExplanationf("Template expression must be a single value-producing action: `%s`", expr).
			WithHint("if/range/with blocks aren't supported; use a single function call that returns a value").
			Err()
	}

	if action.Pipe.IsAssign || len(action.Pipe.Decl) > 0 {
		return nil, errUtils.Build(errUtils.ErrScaffoldExpressionFailed).
			WithExplanationf("Template expression must not declare or assign a variable: `%s`", expr).
			WithHint("Remove the := assignment; the expression itself must produce the list").
			Err()
	}

	return action.Pipe, nil
}

// parseExpressionResult decodes a rendered expression's output (always
// JSON -- see RenderAnswersListExpression) into its resolved list of string
// values.
func parseExpressionResult(rendered, expr string) ([]string, error) {
	var result []string
	if err := json.Unmarshal([]byte(rendered), &result); err != nil {
		return nil, errUtils.Build(errUtils.ErrScaffoldExpressionFailed).
			WithCause(err).
			WithExplanationf("Template expression did not resolve to a list of strings: `%s`", expr).
			WithHint("Ensure the expression's function returns a list of strings").
			Err()
	}
	return result, nil
}

// parseExpressionResultAny decodes a rendered expression's output (always
// JSON -- see RenderAnswersExpression) into whatever value shape it
// produces: string, bool, float64, []any, map[string]any, or nil.
func parseExpressionResultAny(rendered, expr string) (any, error) {
	var result any
	if err := json.Unmarshal([]byte(rendered), &result); err != nil {
		return nil, errUtils.Build(errUtils.ErrScaffoldExpressionFailed).
			WithCause(err).
			WithExplanationf("Template expression did not resolve to a valid value: `%s`", expr).
			WithHint("Ensure the expression's function returns a JSON-encodable value").
			Err()
	}
	return result, nil
}
