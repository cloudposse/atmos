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
// through object literals, merge(...), conditionals, and local.* references. A value whose sensitive part
// cannot be addressed by object keys, such as a list element, a for expression, an index, or the result of
// another function, is marked sensitive as a whole. Only paths that exist in the evaluated inputs are returned.
//
//nolint:lintroller // Plugin evaluator code cannot import the host performance tracker.
func SensitiveInputs(fsys afero.Fs, files []string, inputs map[string]any) [][]string {
	var found [][]string
	bodies := make(map[string]*hclsyntax.Body)
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
		bodies[filepath.Clean(file)] = body
	}
	for file, body := range bodies {
		found = append(found, scanFile(body, file, bodies)...)
	}
	return existingPaths(uniquePaths(found), inputs)
}

// scanFile finds the sensitive paths declared by one file's inputs attribute.
func scanFile(body *hclsyntax.Body, file string, bodies map[string]*hclsyntax.Body) [][]string {
	attribute := body.Attributes[inputsName]
	if attribute == nil {
		return nil
	}
	scanner := &sensitivityScanner{locals: localExpressions(body), includes: exposedIncludes(body, file, bodies)}
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
	locals   map[string]hclsyntax.Expression
	includes map[string]map[string]hclsyntax.Expression
	paths    [][]string
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
		s.walkReference(node, prefix, depth)
	default:
		// A path is a list of object keys, so a sensitive value inside a list, a for expression, an
		// index, or a template cannot be addressed on its own. Mark the whole value instead of dropping it.
		if s.containsSensitive(expression, depth) {
			s.paths = append(s.paths, slices.Clone(prefix))
		}
	}
}

// containsSensitive reports whether expression calls sensitive(...) anywhere, directly or through the
// locals it references.
func (s *sensitivityScanner) containsSensitive(expression hclsyntax.Expression, depth int) bool {
	if depth >= maxLocalDepth {
		// The remaining expression may depend on a sensitive local. Fail closed
		// when the bound (including a cycle) prevents us from inspecting it.
		return true
	}
	found := false
	_ = hclsyntax.VisitAll(expression, func(node hclsyntax.Node) hcl.Diagnostics {
		if found {
			return nil
		}
		switch typed := node.(type) {
		case *hclsyntax.FunctionCallExpr:
			found = typed.Name == "sensitive"
		case *hclsyntax.ScopeTraversalExpr:
			found = s.referenceContainsSensitive(typed, depth)
		}
		return nil
	})
	return found
}

// localContainsSensitive reports whether the local a traversal starts from contains sensitive(...).
func (s *sensitivityScanner) referenceContainsSensitive(traversal *hclsyntax.ScopeTraversalExpr, depth int) bool {
	current, locals, _, found := s.reference(traversal.Traversal)
	if !found {
		return false
	}
	previous := s.locals
	s.locals = locals
	defer func() { s.locals = previous }()
	return s.containsSensitive(current, depth+1)
}

// walkObject records the sensitive paths below each key of an object literal.
func (s *sensitivityScanner) walkObject(object *hclsyntax.ObjectConsExpr, prefix []string, depth int) {
	for _, item := range object.Items {
		if key, ok := staticKey(item.KeyExpr); ok {
			s.walk(item.ValueExpr, append(slices.Clone(prefix), key), depth)
		} else {
			// A computed key cannot be represented in a sensitivity path.
			s.paths = append(s.paths, slices.Clone(prefix))
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
	default:
		// Any other function, such as jsonencode(sensitive(...)), can carry a sensitive argument into its
		// result; its paths are unknown, so the whole result is sensitive.
		for _, argument := range call.Args {
			if s.containsSensitive(argument, depth) {
				s.paths = append(s.paths, slices.Clone(prefix))
				return
			}
		}
	}
}

// walkLocal follows local.name and local.name.key.key to the expression that defines it.
func (s *sensitivityScanner) walkReference(traversal *hclsyntax.ScopeTraversalExpr, prefix []string, depth int) {
	current, locals, rest, found := s.reference(traversal.Traversal)
	if !found {
		return
	}
	if depth >= maxLocalDepth {
		s.paths = append(s.paths, slices.Clone(prefix))
		return
	}
	previous := s.locals
	s.locals = locals
	defer func() { s.locals = previous }()
	for _, step := range rest {
		key, ok := traversalKey(step)
		if !ok {
			// An index the scan cannot follow: keep the value sensitive if any of it may be.
			if s.containsSensitive(current, depth+1) {
				s.paths = append(s.paths, slices.Clone(prefix))
			}
			return
		}
		child, sensitive := descend(current, key)
		if sensitive {
			s.paths = append(s.paths, slices.Clone(prefix))
			return
		}
		if child == nil {
			// Composed locals cannot always be descended by key. Their selected
			// result may still carry sensitivity from any of their arguments.
			if s.containsSensitive(current, depth+1) {
				s.paths = append(s.paths, slices.Clone(prefix))
			}
			return
		}
		current = child
	}
	s.walk(current, prefix, depth+1)
}

// reference selects a local in this file or in an exposed include.
func (s *sensitivityScanner) reference(traversal hcl.Traversal) (hclsyntax.Expression, map[string]hclsyntax.Expression, hcl.Traversal, bool) {
	if len(traversal) < 2 {
		return nil, nil, nil, false
	}
	if traversal.RootName() == localName {
		name, ok := traversal[1].(hcl.TraverseAttr)
		if !ok {
			return nil, nil, nil, false
		}
		value, found := s.locals[name.Name]
		return value, s.locals, traversal[2:], found
	}
	if traversal.RootName() == "include" && len(traversal) >= 4 {
		alias, aliasOK := traversal[1].(hcl.TraverseAttr)
		section, sectionOK := traversal[2].(hcl.TraverseAttr)
		name, nameOK := traversal[3].(hcl.TraverseAttr)
		if aliasOK && sectionOK && nameOK && section.Name == "locals" {
			locals := s.includes[alias.Name]
			value, found := locals[name.Name]
			return value, locals, traversal[4:], found
		}
	}
	return nil, nil, nil, false
}

// exposedIncludes associates an include alias with the parsed file's locals.
func exposedIncludes(body *hclsyntax.Body, file string, bodies map[string]*hclsyntax.Body) map[string]map[string]hclsyntax.Expression {
	result := make(map[string]map[string]hclsyntax.Expression)
	for _, block := range body.Blocks {
		if block.Type != "include" || len(block.Labels) == 0 {
			continue
		}
		expose, ok := block.Body.Attributes["expose"]
		if !ok {
			continue
		}
		value, diagnostics := expose.Expr.Value(nil)
		if diagnostics.HasErrors() || !value.IsKnown() || value.IsNull() || !value.Type().Equals(cty.Bool) || !value.True() {
			continue
		}
		path, ok := includePath(block.Body.Attributes["path"])
		if !ok {
			continue
		}
		for dir := filepath.Dir(file); ; dir = filepath.Dir(dir) {
			candidate := filepath.Clean(path)
			if !filepath.IsAbs(path) {
				candidate = filepath.Join(dir, path)
			}
			if included := bodies[candidate]; included != nil {
				result[block.Labels[0]] = localExpressions(included)
				break
			}
			if parent := filepath.Dir(dir); parent == dir || filepath.IsAbs(path) {
				break
			}
		}
	}
	return result
}

func includePath(attribute *hclsyntax.Attribute) (string, bool) {
	if attribute == nil {
		return "", false
	}
	expression := attribute.Expr
	if call, ok := expression.(*hclsyntax.FunctionCallExpr); ok && call.Name == "find_in_parent_folders" && len(call.Args) > 0 {
		expression = call.Args[0]
	}
	value, diagnostics := expression.Value(nil)
	if diagnostics.HasErrors() || !value.IsKnown() || value.IsNull() || !value.Type().Equals(cty.String) {
		return "", false
	}
	return value.AsString(), true
}

// traversalKey returns the object key of local.name.key and local.name["key"].
func traversalKey(step hcl.Traverser) (string, bool) {
	switch typed := step.(type) {
	case hcl.TraverseAttr:
		return typed.Name, true
	case hcl.TraverseIndex:
		if typed.Key.IsKnown() && !typed.Key.IsNull() && typed.Key.Type().Equals(cty.String) {
			return typed.Key.AsString(), true
		}
	}
	return "", false
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
