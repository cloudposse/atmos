package template

import (
	"text/template/parse"

	"github.com/cloudposse/atmos/pkg/perf"
)

// StaticFieldRefs returns root-relative dependencies without executing functions.
// Function names are resolved by the evaluator, not by this structural analysis.
// Dynamic scope, whole-root access, and named template calls require full evaluation.
func StaticFieldRefs(input string) ([]FieldRef, bool) {
	defer perf.Track(nil, "template.StaticFieldRefs")()

	tree := parse.New("requirements")
	tree.Mode = parse.SkipFuncCheck
	if _, err := tree.Parse(input, "{{", "}}", make(map[string]*parse.Tree)); err != nil {
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
