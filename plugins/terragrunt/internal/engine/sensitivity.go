package engine

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/spf13/afero"
	"github.com/zclconf/go-cty/cty"
)

const (
	// Bound on how many local.* references the scan follows, which also stops cycles.
	maxLocalDepth = 8
	inputsName    = "inputs"
	localName     = "local"
)

// SensitiveInputs returns the paths of the unit's inputs that the unit declares sensitive. A path is a list
// of object keys from the top of the inputs; an empty path means every input.
//
// Terragrunt converts the evaluated inputs to plain values and drops their marks, so the evaluated result
// cannot tell which inputs were sensitive. The unit's own HCL does: this reads the `inputs` attribute of the
// unit and of every file it includes, and marks the paths whose value is written as sensitive(...), including
// through object literals, merge(...), conditionals, and local.* references. Values that become sensitive in
// other ways, such as the output of a function applied to a sensitive local, are not tracked. Only paths that
// exist in the evaluated inputs are returned.
//
//nolint:lintroller // Plugin evaluator code cannot import the host performance tracker.
func SensitiveInputs(fsys afero.Fs, files []string, inputs map[string]any) [][]string {
	var found [][]string
	for _, file := range files {
		if filepath.Ext(file) != ".hcl" {
			continue
		}
		content, err := afero.ReadFile(fsys, file)
		if err != nil {
			continue
		}
		parsed, diags := hclsyntax.ParseConfig(content, file, hcl.InitialPos)
		if diags.HasErrors() {
			continue
		}
		body, ok := parsed.Body.(*hclsyntax.Body)
		if !ok {
			continue
		}
		found = append(found, scanFile(body)...)
	}
	return existingPaths(uniquePaths(found), inputs)
}

// scanFile finds the sensitive paths declared by one file's inputs attribute.
func scanFile(body *hclsyntax.Body) [][]string {
	attribute := body.Attributes[inputsName]
	if attribute == nil {
		return nil
	}
	scanner := &sensitivityScanner{locals: localExpressions(body)}
	scanner.walk(attribute.Expr, nil, 0)
	return scanner.paths
}

// localExpressions indexes the attributes of the file's locals blocks by name.
func localExpressions(body *hclsyntax.Body) map[string]hclsyntax.Expression {
	locals := map[string]hclsyntax.Expression{}
	for _, block := range body.Blocks {
		if block.Type != "locals" {
			continue
		}
		for name, attribute := range block.Body.Attributes {
			locals[name] = attribute.Expr
		}
	}
	return locals
}

type sensitivityScanner struct {
	locals map[string]hclsyntax.Expression
	paths  [][]string
}

// walk records the paths below prefix that expression marks sensitive.
func (s *sensitivityScanner) walk(expression hclsyntax.Expression, prefix []string, depth int) {
	switch node := expression.(type) {
	case *hclsyntax.ParenthesesExpr:
		s.walk(node.Expression, prefix, depth)
	case *hclsyntax.TemplateWrapExpr:
		s.walk(node.Wrapped, prefix, depth)
	case *hclsyntax.FunctionCallExpr:
		s.walkCall(node, prefix, depth)
	case *hclsyntax.ObjectConsExpr:
		s.walkObject(node, prefix, depth)
	case *hclsyntax.ConditionalExpr:
		s.walk(node.TrueResult, prefix, depth)
		s.walk(node.FalseResult, prefix, depth)
	case *hclsyntax.ScopeTraversalExpr:
		s.walkLocal(node, prefix, depth)
	}
}

// walkObject records the sensitive paths below each key of an object literal.
func (s *sensitivityScanner) walkObject(object *hclsyntax.ObjectConsExpr, prefix []string, depth int) {
	for _, item := range object.Items {
		if key, ok := staticKey(item.KeyExpr); ok {
			s.walk(item.ValueExpr, append(slices.Clone(prefix), key), depth)
		}
	}
}

// staticKey returns the key of an object literal item when it is a known string.
func staticKey(expression hclsyntax.Expression) (string, bool) {
	key, diags := expression.Value(nil)
	if diags.HasErrors() || !key.IsKnown() || key.IsNull() || !key.Type().Equals(cty.String) {
		return "", false
	}
	return key.AsString(), true
}

func (s *sensitivityScanner) walkCall(call *hclsyntax.FunctionCallExpr, prefix []string, depth int) {
	switch call.Name {
	case "sensitive":
		s.paths = append(s.paths, slices.Clone(prefix))
	case "merge":
		for _, argument := range call.Args {
			s.walk(argument, prefix, depth)
		}
	}
}

// walkLocal follows local.name and local.name.key.key to the expression that defines it.
func (s *sensitivityScanner) walkLocal(traversal *hclsyntax.ScopeTraversalExpr, prefix []string, depth int) {
	if depth >= maxLocalDepth || traversal.Traversal.RootName() != localName || len(traversal.Traversal) < 2 {
		return
	}
	first, ok := traversal.Traversal[1].(hcl.TraverseAttr)
	if !ok {
		return
	}
	current, found := s.locals[first.Name]
	if !found {
		return
	}
	for _, step := range traversal.Traversal[2:] {
		attribute, ok := step.(hcl.TraverseAttr)
		if !ok {
			return
		}
		var sensitive bool
		if current, sensitive = descend(current, attribute.Name); sensitive {
			s.paths = append(s.paths, slices.Clone(prefix))
			return
		}
		if current == nil {
			return
		}
	}
	s.walk(current, prefix, depth+1)
}

// Descend returns the expression of one key of an object literal. A sensitive wrapper call makes every key
// sensitive, which it reports instead of an expression.
func descend(expression hclsyntax.Expression, key string) (child hclsyntax.Expression, sensitive bool) {
	switch node := expression.(type) {
	case *hclsyntax.ParenthesesExpr:
		return descend(node.Expression, key)
	case *hclsyntax.FunctionCallExpr:
		return nil, node.Name == "sensitive"
	case *hclsyntax.ObjectConsExpr:
		for _, item := range node.Items {
			if name, ok := staticKey(item.KeyExpr); ok && name == key {
				return item.ValueExpr, false
			}
		}
	}
	return nil, false
}

func uniquePaths(paths [][]string) [][]string {
	slices.SortFunc(paths, func(a, b []string) int { return strings.Compare(strings.Join(a, "\x00"), strings.Join(b, "\x00")) })
	return slices.CompactFunc(paths, slices.Equal[[]string])
}

// existingPaths keeps the paths that lead to a value in inputs. The empty path always exists.
func existingPaths(paths [][]string, inputs map[string]any) [][]string {
	var kept [][]string
	for _, path := range paths {
		if pathExists(any(inputs), path) {
			kept = append(kept, path)
		}
	}
	return kept
}

func pathExists(value any, path []string) bool {
	if len(path) == 0 {
		return true
	}
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	child, found := object[path[0]]
	return found && pathExists(child, path[1:])
}
