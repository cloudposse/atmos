package engine

import (
	"sort"
	"strconv"
	"strings"
	"text/template"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// axisListMarker prefixes axisList's encoded form. Its mere presence (not
// its absence, and not what follows it byte-for-byte) is what
// unambiguously signals "this is axisList's own encoding" to
// parseAxisExpressionResult, regardless of what the elements themselves
// contain.
const axisListMarker = "\x1f"

// axisList is []string with a custom String(): when a matrix axis
// expression's entire body is one bare function call (e.g. "{{ keys
// answers.environments }}"), Go's text/template stringifies the result via
// fmt's Stringer interface if present, so returning axisList instead of a
// plain []string makes that stringification unambiguous and losslessly
// reversible by parseAxisExpressionResult -- unlike []string's default %v
// formatting ("[a b c]", space-joined), which can't distinguish "one value
// containing a space" from "two values". Ranging over axisList (e.g. "{{
// range keys answers.environments }}") behaves identically to []string,
// since String() is only consulted when the value itself is the thing being
// printed.
type axisList []string

// String encodes v as axisListMarker followed by each element in
// length-prefixed ("netstring") form: "<byte-length>:<bytes>" for every
// element, back to back, with no separator between entries (none is
// needed -- each element's own length prefix says exactly how many bytes
// to consume next). This is unambiguous for any element content
// whatsoever, including an empty string, multiple empty strings, or an
// element that happens to contain axisListMarker itself -- unlike a
// plain separator-joined encoding, which cannot distinguish an empty
// list from a single empty-string element, and would incorrectly split
// an element containing the separator character.
func (v axisList) String() string {
	var b strings.Builder
	b.WriteString(axisListMarker)
	for _, s := range v {
		b.WriteString(strconv.Itoa(len(s)))
		b.WriteByte(':')
		b.WriteString(s)
	}
	return b.String()
}

// keysFunc is the "keys" template function available to scaffold templates
// and matrix axis expressions. With no extra argument it returns v's
// top-level keys, sorted for deterministic matrix expansion. With a
// nestedKey argument, it treats v as a map of maps and instead collects
// nestedKey's own keys from every one of v's values, flattening and
// deduplicating across all of them -- e.g. keys(answers.environments,
// "regions") returns every region that appears in any environment.
func keysFunc(v any, nestedKey ...string) (axisList, error) {
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
// into a list of values.
//
// The primary, unambiguous path: decodeAxisList recognizes the expression's
// result as an axisList (e.g. keys' return value) stringified via its
// String() method, and losslessly recovers every value exactly as computed,
// including an empty list, a single empty-string value, or a value that
// itself contains whitespace or the marker byte -- see axisList.String()'s
// and decodeAxisList's doc comments.
//
// Fallback for anything else (a custom function that doesn't return
// axisList): tolerant of Go's default %v slice formatting ("[a b c]") as
// well as a plain whitespace-separated list without brackets, so a template
// author isn't tied to axisList specifically. This fallback can't
// distinguish "one value containing a space" from "two values" -- but it
// only applies when the result isn't axisList's own encoding, i.e. the
// expression didn't go through axisList's String() in the first place.
func parseAxisExpressionResult(rendered string) []string {
	if result, ok := decodeAxisList(rendered); ok {
		return result
	}

	trimmed := strings.TrimSpace(rendered)
	trimmed = strings.TrimPrefix(trimmed, "[")
	trimmed = strings.TrimSuffix(trimmed, "]")
	if trimmed == "" {
		return []string{}
	}
	return strings.Fields(trimmed)
}

// decodeAxisList decodes axisList.String()'s length-prefixed encoding
// back into its original elements. ok is false if rendered doesn't start
// with axisListMarker (not axisList's encoding at all) or is malformed
// (which should never happen for genuine axisList output -- only
// possible if something else coincidentally produced a string starting
// with the marker byte, astronomically unlikely for real template
// output).
func decodeAxisList(rendered string) (result []string, ok bool) {
	rest, hasMarker := strings.CutPrefix(rendered, axisListMarker)
	if !hasMarker {
		return nil, false
	}

	result = []string{}
	for len(rest) > 0 {
		idx := strings.IndexByte(rest, ':')
		if idx < 0 {
			return nil, false
		}
		n, err := strconv.Atoi(rest[:idx])
		if err != nil || n < 0 || n > len(rest)-idx-1 {
			return nil, false
		}
		rest = rest[idx+1:]
		result = append(result, rest[:n])
		rest = rest[n:]
	}
	return result, true
}
