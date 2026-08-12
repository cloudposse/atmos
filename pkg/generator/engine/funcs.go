package engine

import (
	"sort"
	"strings"
	"text/template"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// keysFunc is the "keys" template function available to scaffold templates
// and matrix axis expressions. With no extra argument it returns v's
// top-level keys, sorted for deterministic matrix expansion. With a
// nestedKey argument, it treats v as a map of maps and instead collects
// nestedKey's own keys from every one of v's values, flattening and
// deduplicating across all of them -- e.g. keys(answers.environments,
// "regions") returns every region that appears in any environment.
func keysFunc(v any, nestedKey ...string) ([]string, error) {
	m, err := toStringKeyedMap(v)
	if err != nil {
		return nil, err
	}

	if len(nestedKey) == 0 {
		return sortedKeys(m), nil
	}

	return flattenNestedKeys(m, nestedKey[0]), nil
}

// toStringKeyedMap asserts v is a map keyed by string, the shape scaffold
// answers (YAML-decoded) always use.
func toStringKeyedMap(v any) (map[string]interface{}, error) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, errUtils.Build(errUtils.ErrScaffoldKeysFuncNotMap).
			WithExplanationf("keys: expected a map, got %T", v).
			Err()
	}
	return m, nil
}

// sortedKeys returns m's keys in sorted order.
func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// flattenNestedKeys collects nestedKey's own keys from every value in m,
// flattening and deduplicating across all of them. A value that isn't a map,
// or doesn't have nestedKey, is silently skipped -- the same tolerance for
// heterogeneous answer shapes the rest of matrix axis resolution already has.
func flattenNestedKeys(m map[string]interface{}, nestedKey string) []string {
	seen := make(map[string]struct{})
	for _, entry := range m {
		entryMap, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		nested, ok := entryMap[nestedKey]
		if !ok {
			continue
		}
		nestedMap, ok := nested.(map[string]interface{})
		if !ok {
			continue
		}
		for k := range nestedMap {
			seen[k] = struct{}{}
		}
	}

	result := make([]string, 0, len(seen))
	for k := range seen {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}

// RenderMatrixAxisExpression renders a Go-template matrix-axis expression
// (e.g. "{{ keys answers.environments }}") against answers and returns its
// resolved list of string values. Satisfies the AxisRenderer type ExpandMatrix
// accepts, giving matrix axis expressions the same Gomplate, Sprig, and
// custom (including keys) FuncMap scaffold templates already use. Delimiters
// is the scaffold's active left/right template delimiter pair (an
// invalid/empty value defaults to "{{"/"}}"), so a matrix axis expression
// honors a scaffold's own spec.delimiters override exactly like target: and
// file content rendering already do via ProcessTemplateWithDelimiters.
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

	if len(delimiters) != 2 {
		delimiters = []string{defaultLeftDelimiter, defaultRightDelimiter}
	}

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

	return parseBracketedList(result.String()), nil
}

// parseBracketedList parses a rendered axis expression's text output into a
// list of values. Tolerant of Go's default %v slice formatting ("[a b c]",
// what {{ keys ... }} renders as) as well as a plain whitespace-separated
// list without brackets, so a template author isn't tied to one specific
// function's output shape.
//
// Known constraint: splitting on whitespace cannot distinguish "one value
// containing a space" from "two values" -- an individual resolved value with
// whitespace in it (e.g. a map key with a space) would be split incorrectly.
// This is accepted rather than worked around: matrix axis values are always
// identifier-like strings in practice (environment names, region codes --
// infrastructure taxonomies that don't contain whitespace), and Go's
// text/template always stringifies a returned []string via this same
// space-joined %v format, so switching the delimiter here (e.g. to
// newlines) wouldn't help without also requiring template authors to write
// {{ range keys answers.environments }}{{ . }}{{ "\n" }}{{ end }} instead of
// the single-call {{ keys answers.environments }} syntax this feature is
// designed around (see https://github.com/orgs/cloudposse/discussions/126).
func parseBracketedList(rendered string) []string {
	trimmed := strings.TrimSpace(rendered)
	trimmed = strings.TrimPrefix(trimmed, "[")
	trimmed = strings.TrimSuffix(trimmed, "]")
	if trimmed == "" {
		return []string{}
	}
	return strings.Fields(trimmed)
}
