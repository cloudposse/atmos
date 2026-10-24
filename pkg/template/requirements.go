package template

import (
	"slices"
	"text/template/parse"

	"github.com/cloudposse/atmos/pkg/perf"
)

// StaticFieldRefs returns root-relative dependencies without executing functions.
// Function names are resolved by the evaluator, not by this structural analysis.
// Dynamic scope, whole-root access, and named template calls require full evaluation.
// An optional delimiter pair uses the same syntax as the template evaluator.
func StaticFieldRefs(input string, delimiters ...string) ([]FieldRef, bool) {
	defer perf.Track(nil, "template.StaticFieldRefs")()

	tree, ok := parseRequirements(input, delimiters)
	if !ok {
		return nil, false
	}
	var refs []FieldRef
	static := true
	walkAST(tree.Root, func(node parse.Node) {
		switch n := node.(type) {
		case *parse.FieldNode:
			refs = append(refs, FieldRef{Path: n.Ident})
		case *parse.DotNode, *parse.RangeNode, *parse.WithNode, *parse.TemplateNode, *parse.VariableNode:
			static = false
		}
	})
	return refs, static
}

// StaticFunctionNames returns the distinct names of the functions a template calls, in order of first
// appearance, without executing anything. Callers decide which functions are acceptable. The second
// result is false when the template does not parse. An optional delimiter pair uses the same syntax as
// the template evaluator.
func StaticFunctionNames(input string, delimiters ...string) ([]string, bool) {
	defer perf.Track(nil, "template.StaticFunctionNames")()

	tree, ok := parseRequirements(input, delimiters)
	if !ok {
		return nil, false
	}
	var names []string
	walkAST(tree.Root, func(node parse.Node) {
		if identifier, isFunction := node.(*parse.IdentifierNode); isFunction && !slices.Contains(names, identifier.Ident) {
			names = append(names, identifier.Ident)
		}
	})
	return names, true
}

// parseRequirements parses a template without checking that its functions exist.
func parseRequirements(input string, delimiters []string) (*parse.Tree, bool) {
	left, right := "{{", "}}"
	if len(delimiters) != 0 {
		if len(delimiters) != 2 || delimiters[0] == "" || delimiters[1] == "" {
			return nil, false
		}
		left, right = delimiters[0], delimiters[1]
	}
	tree := parse.New("requirements")
	tree.Mode = parse.SkipFuncCheck
	if _, err := tree.Parse(input, left, right, make(map[string]*parse.Tree)); err != nil {
		return nil, false
	}
	return tree, true
}
