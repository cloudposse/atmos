package engine

import (
	"fmt"
	"sort"
	"strings"
	"text/template"
	"text/template/parse"

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

// FileContextKey is the reserved answers key the current discovered file's
// own path travels under, mirroring MatrixKey: ProcessTemplateWithDelimiters
// hoists it onto the template root as "file", so a directory-level spec.files
// entry (path: a glob matching many discovered files) can write
// {{ .file.RelPath }} in its target: to differentiate one matched file's
// output from another's -- see FileContext.
const FileContextKey = "File"

// FileContext is the per-file template data hoisted under FileContextKey.
// Path is the file's own path as discovered in the template's source tree.
// RelPath is Path with the matching spec.files[] entry's glob literal-prefix
// stripped (see pkg/utils.WildcardRelPath) -- equal to Path when that
// entry's path has no glob metacharacter, since there's no literal prefix to
// strip. This is what lets a single directory-level matrix entry (e.g.
// path: "components/**") preserve each matched file's own relative position
// under a combination-specific target, e.g.
// target: "environments/{{ .matrix.env }}/{{ .file.RelPath }}".
type FileContext struct {
	Path    string
	RelPath string
}

// TargetReferencesFileContext reports whether target's parsed template
// contains a genuine field-access node reading .file.Path or .file.RelPath
// -- the two exported fields of FileContext, hoisted onto the template root
// as "file" by ProcessTemplateWithDelimiters -- under the given delimiters.
//
// This walks target's parsed AST rather than doing a raw
// strings.Contains(target, ".file.") substring search (the bug this
// function replaces): a substring match would also accept a target that
// merely contains the literal text ".file." outside any template action
// (e.g. an output filename like "output.file.txt"), or an invalid field
// reference like ".file.Unknown" that can never actually differentiate
// matched files -- both would let a directory-matrix target: back into
// production that still collides on the *first* two matched files' writes.
// See validateDirectoryMatrixTargetsDifferentiate in pkg/generator/ui,
// which calls this before any file in the run is written.
func TargetReferencesFileContext(target string, delimiters []string) (bool, error) {
	defer perf.Track(nil, "engine.TargetReferencesFileContext")()

	delimiters = defaultDelimiters(delimiters)

	tmpl, err := template.New("target-file-context-check").
		Delims(delimiters[0], delimiters[1]).
		Funcs(buildTemplateFuncMap(nil)).
		Parse(target)
	if err != nil {
		return false, errUtils.Build(errUtils.ErrScaffoldExpressionFailed).
			WithCause(err).
			WithExplanationf("Failed to parse target template: `%s`", target).
			WithHint("Check template syntax in target:").
			Err()
	}

	return nodeReferencesFileContext(tmpl.Root), nil
}

// nodeReferencesFileContext recursively walks a parsed template's node tree
// looking for a field-access node whose identifier chain resolves to
// isFileContextIdent -- i.e. a genuine ".file.Path" or ".file.RelPath"
// reference, as opposed to a lexical match on those characters anywhere in
// the source text. Node kinds with no children of interest (text, numbers,
// strings, booleans, nil, dot, variables) fall through to the final "return
// false" -- they can never themselves be, or contain, a field access.
func nodeReferencesFileContext(n parse.Node) bool {
	switch node := n.(type) {
	case *parse.ListNode:
		if node == nil {
			return false
		}
		for _, c := range node.Nodes {
			if nodeReferencesFileContext(c) {
				return true
			}
		}
	case *parse.ActionNode:
		return nodeReferencesFileContext(node.Pipe)
	case *parse.PipeNode:
		if node == nil {
			return false
		}
		for _, cmd := range node.Cmds {
			if nodeReferencesFileContext(cmd) {
				return true
			}
		}
	case *parse.CommandNode:
		for _, arg := range node.Args {
			if nodeReferencesFileContext(arg) {
				return true
			}
		}
	case *parse.FieldNode:
		return isFileContextIdent(node.Ident)
	case *parse.ChainNode:
		if isFileContextIdent(node.Field) {
			return true
		}
		return nodeReferencesFileContext(node.Node)
	case *parse.IfNode:
		return nodeReferencesFileContext(&node.BranchNode)
	case *parse.RangeNode:
		return nodeReferencesFileContext(&node.BranchNode)
	case *parse.WithNode:
		return nodeReferencesFileContext(&node.BranchNode)
	case *parse.BranchNode:
		if node.Pipe != nil && nodeReferencesFileContext(node.Pipe) {
			return true
		}
		if node.List != nil && nodeReferencesFileContext(node.List) {
			return true
		}
		if node.ElseList != nil && nodeReferencesFileContext(node.ElseList) {
			return true
		}
	case *parse.TemplateNode:
		if node.Pipe != nil {
			return nodeReferencesFileContext(node.Pipe)
		}
	}
	return false
}

// isFileContextIdent reports whether ident is a field-access chain rooted at
// "file" (the name ProcessTemplateWithDelimiters hoists FileContext onto)
// selecting Path or RelPath -- FileContext's two exported fields -- allowing
// (but not requiring) further chained selectors after that.
func isFileContextIdent(ident []string) bool {
	return len(ident) >= 2 && ident[0] == "file" && (ident[1] == "Path" || ident[1] == "RelPath")
}

// answersPrefix is the required prefix for a dynamic matrix axis source, a
// root reference into the answers map -- see resolveMatrixAxis.
const answersPrefix = "answers."

// AxisRenderer renders a Go-template matrix-axis expression (e.g. "{{
// collectKeys answers.environments }}") against answers into its resolved
// list of string values, honoring the scaffold's own spec.delimiters
// override. A nil renderer disables template-expression axes, keeping
// ExpandMatrix's core algorithm free of any templating dependency -- see
// Processor.RenderAnswersListExpression in funcs.go for the real
// implementation.
type AxisRenderer func(expr string, answers map[string]interface{}, delimiters []string) ([]string, error)

// defaultDelimiters returns delimiters unchanged when it's a valid
// two-element pair with both sides non-empty, otherwise the default
// "{{"/"}}". Named generically (not "axis") because it also backs
// RenderAnswersListExpression in funcs.go, shared by matrix axis and field
// Options expressions alike. An empty side is rejected rather than passed
// through: an empty left delimiter would make strings.Contains(v,
// delimiters[0]) match every axis value in resolveMatrixAxis, and
// text/template.Delims treats an empty argument as "use the default" for
// that side only, producing a mismatched half-custom/half-default pair.
func defaultDelimiters(delimiters []string) []string {
	if len(delimiters) != 2 || delimiters[0] == "" || delimiters[1] == "" {
		return []string{defaultLeftDelimiter, defaultRightDelimiter}
	}
	return delimiters
}

// ExpandMatrix resolves a FileSpec's Matrix axes against answers and returns
// their full Cartesian product as one row (map[axis]value) per combination,
// expanded in a sorted, deterministic order per axis -- the same behavior
// pkg/workflow's own matrix step expansion has, so regenerating the same
// answers produces the same file set, in the same order, every time.
// Delimiters is the active scaffold's left/right template delimiter pair
// (see defaultDelimiters for what an empty/nil value defaults to).
func ExpandMatrix(matrix map[string]any, answers map[string]interface{}, render AxisRenderer, delimiters []string) ([]map[string]string, error) {
	defer perf.Track(nil, "engine.ExpandMatrix")()

	delimiters = defaultDelimiters(delimiters)

	resolved := make(map[string][]string, len(matrix))
	for axis, raw := range matrix {
		values, err := resolveMatrixAxis(axis, raw, answers, render, delimiters)
		if err != nil {
			return nil, err
		}
		resolved[axis] = values
	}
	return cartesianProduct(resolved), nil
}

// resolveMatrixAxis resolves one axis's declared value into its list of
// string values: a literal list is used as-is; a string is either a
// Go-template expression (containing delimiters[0], rendered via render,
// e.g. to compute a list from nested/structured answer data with keys) or a
// dot-path into answers.* (validated at load time to require that prefix)
// that must resolve to an already list-shaped value.
func resolveMatrixAxis(axis string, raw any, answers map[string]interface{}, render AxisRenderer, delimiters []string) ([]string, error) {
	switch v := raw.(type) {
	case []string:
		return v, nil
	case []any:
		values := make([]string, len(v))
		for i, item := range v {
			s, err := toString(axis, i, item)
			if err != nil {
				return nil, err
			}
			values[i] = s
		}
		return values, nil
	case string:
		if strings.Contains(v, delimiters[0]) {
			return resolveMatrixAxisExpression(axis, v, answers, render, delimiters)
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
func resolveMatrixAxisExpression(axis, expr string, answers map[string]interface{}, render AxisRenderer, delimiters []string) ([]string, error) {
	if render == nil {
		return nil, errUtils.Build(errUtils.ErrScaffoldMatrixExpressionFailed).
			WithExplanationf("matrix axis %q is a template expression, but no template renderer is available", axis).
			Err()
	}
	return render(expr, answers, delimiters)
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
			s, err := toString(axis, i, item)
			if err != nil {
				return nil, err
			}
			values[i] = s
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
// Rejects non-scalar values (maps, slices, structs), which become illegal
// path/filename characters once used as a path segment.
func toString(axis string, index int, value any) (string, error) {
	switch value.(type) {
	case nil, string, bool,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return fmt.Sprint(value), nil
	default:
		return "", errUtils.Build(errUtils.ErrScaffoldMatrixAxisValueNotScalar).
			WithExplanationf("matrix axis %q value at index %d is a %T, not a scalar", axis, index, value).
			WithHint("Matrix axis values must be strings, numbers, or booleans -- not maps or lists").
			Err()
	}
}

// cartesianProduct expands axes into their full Cartesian product, one row
// (map[axis]value) per combination, iterating axes in sorted order so
// regenerating the same answers produces the same file set every time --
// mirroring pkg/workflow/control_matrix.go's expandMatrix, intentionally
// parallel rather than imported to keep this package dependency-free.
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
