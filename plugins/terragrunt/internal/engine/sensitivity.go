package engine

import (
	"errors"
	"math/big"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/config"
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
// unit and of every file it includes, and marks the paths whose value is written as `sensitive`, including
// through object literals, `merge`, conditionals, and local.* references. A value whose sensitive part
// cannot be addressed by object keys, such as a list element, a for expression, an index, or the result of
// another function, is marked sensitive as a whole. Only paths that exist in the evaluated inputs are returned.
//
//nolint:lintroller // Plugin evaluator code cannot import the host performance tracker.
func SensitiveInputs(fsys afero.Fs, files []string, inputs map[string]any) [][]string {
	return sensitiveInputs(fsys, files, inputs, "", nil)
}

// sensitiveInputs binds exposed aliases to the paths from this evaluation.
func sensitiveInputs(fsys afero.Fs, files []string, inputs map[string]any, unit string, includes config.IncludeConfigsMap) [][]string {
	var found [][]string
	bodies := make(map[string]*hclsyntax.Body)
	for _, file := range files {
		content, err := afero.ReadFile(fsys, file)
		if errors.Is(err, errNotEvaluated) {
			// The contents Terragrunt evaluated are unknown, so any input may be sensitive.
			found = append(found, nil)
			continue
		}
		if err != nil {
			continue
		}
		// JSON expressions are opaque to this HCL syntax scan. Protect every input
		// until precise JSON sensitivity analysis is available.
		if isJSONConfiguration(file) {
			found = append(found, nil)
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
		var evaluated config.IncludeConfigsMap
		if file == filepath.Clean(unit) {
			evaluated = includes
		}
		found = append(found, scanFile(body, file, bodies, evaluated)...)
	}
	return existingPaths(uniquePaths(found), inputs)
}

// isJSONConfiguration reports whether Terragrunt reads the file as JSON. Every other configuration file is
// HCL, whatever its suffix: an included file does not need to end in `.hcl`.
func isJSONConfiguration(file string) bool {
	return strings.EqualFold(filepath.Ext(file), ".json")
}

// scanFile finds the sensitive paths declared by one file's inputs attribute.
func scanFile(body *hclsyntax.Body, file string, bodies map[string]*hclsyntax.Body, includes config.IncludeConfigsMap) [][]string {
	attribute := body.Attributes[inputsName]
	if attribute == nil {
		return nil
	}
	scanner := &sensitivityScanner{locals: localExpressions(body), includes: exposedIncludes(body, file, bodies, includes)}
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
	includes map[string]exposedInclude
	paths    [][]string
}

// exposedInclude is what an exposed include contributes to the include.<alias> references of the file.
type exposedInclude struct {
	// locals are the locals of the included file.
	locals map[string]hclsyntax.Expression
	// inputs is the inputs attribute of the included file, if any.
	inputs hclsyntax.Expression
	// known is false when the included file is exposed but was not found among the parsed files.
	known bool
}

// unknownExpression stands in for a value that cannot be inspected: it reads as sensitive as a whole.
var unknownExpression hclsyntax.Expression = &hclsyntax.FunctionCallExpr{Name: "sensitive"}

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

// ContainsSensitive reports whether expression calls `sensitive` anywhere, directly or through the
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

// localContainsSensitive reports whether the local a traversal starts from contains `sensitive`.
func (s *sensitivityScanner) referenceContainsSensitive(traversal *hclsyntax.ScopeTraversalExpr, depth int) bool {
	reference, found := s.reference(traversal.Traversal)
	if !found {
		return false
	}
	previous := s.locals
	s.locals = reference.locals
	defer func() { s.locals = previous }()
	return s.containsSensitive(reference.expression, depth+1)
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
		// Any other function, such as jsonencode(`sensitive`), can carry a sensitive argument into its
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
	reference, found := s.reference(traversal.Traversal)
	if !found {
		return
	}
	if depth >= maxLocalDepth {
		s.paths = append(s.paths, slices.Clone(prefix))
		return
	}
	previous := s.locals
	s.locals = reference.locals
	defer func() { s.locals = previous }()
	current := reference.expression
	for _, step := range reference.rest {
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

type sensitivityReference struct {
	expression hclsyntax.Expression
	locals     map[string]hclsyntax.Expression
	rest       hcl.Traversal
}

// reference selects a local in this file, or a local or input of an exposed include. Names are read with
// traversalKey, so local.name and local["name"] select the same value.
func (s *sensitivityScanner) reference(traversal hcl.Traversal) (sensitivityReference, bool) {
	if len(traversal) < 2 {
		return sensitivityReference{}, false
	}
	if traversal.RootName() == localName {
		name, ok := traversalKey(traversal[1])
		if !ok {
			return sensitivityReference{}, false
		}
		value, found := s.locals[name]
		return sensitivityReference{value, s.locals, traversal[2:]}, found
	}
	if traversal.RootName() != "include" || len(traversal) < 3 {
		return sensitivityReference{}, false
	}
	alias, aliasOK := traversalKey(traversal[1])
	section, sectionOK := traversalKey(traversal[2])
	exposed, exposedFound := s.includes[alias]
	if !aliasOK || !sectionOK || !exposedFound {
		return sensitivityReference{}, false
	}
	switch section {
	case "locals":
		if len(traversal) < 4 {
			return sensitivityReference{}, false
		}
		name, nameOK := traversalKey(traversal[3])
		if !nameOK {
			return sensitivityReference{}, false
		}
		value, found := exposed.locals[name]
		return sensitivityReference{value, exposed.locals, traversal[4:]}, found
	case inputsName:
		// A child's merged inputs may omit the parent's (no_merge), so the parent's own expression decides.
		if !exposed.known {
			return sensitivityReference{unknownExpression, nil, nil}, true
		}
		return sensitivityReference{exposed.inputs, exposed.locals, traversal[3:]}, exposed.inputs != nil
	}
	return sensitivityReference{}, false
}

// exposedIncludes associates an include alias with the parsed file's locals and inputs.
func exposedIncludes(body *hclsyntax.Body, file string, bodies map[string]*hclsyntax.Body, evaluated config.IncludeConfigsMap) map[string]exposedInclude {
	result := make(map[string]exposedInclude)
	for _, block := range body.Blocks {
		if block.Type != "include" || len(block.Labels) == 0 {
			continue
		}
		path, ok := exposedIncludePath(block, evaluated)
		if !ok {
			continue
		}
		included := includedBody(path, file, bodies)
		if included == nil {
			result[block.Labels[0]] = exposedInclude{}
			continue
		}
		exposed := exposedInclude{locals: localExpressions(included), known: true}
		if attribute := included.Attributes[inputsName]; attribute != nil {
			exposed.inputs = attribute.Expr
		}
		result[block.Labels[0]] = exposed
	}
	return result
}

// exposedIncludePath reads a statically exposed include's path.
func exposedIncludePath(block *hclsyntax.Block, evaluated config.IncludeConfigsMap) (string, bool) {
	if evaluated != nil {
		include, found := evaluated[block.Labels[0]]
		return include.Path, found && include.Expose != nil && *include.Expose
	}
	expose, ok := block.Body.Attributes["expose"]
	if !ok {
		return "", false
	}
	value, diagnostics := expose.Expr.Value(nil)
	if diagnostics.HasErrors() || !value.IsKnown() || value.IsNull() || !value.Type().Equals(cty.Bool) || !value.True() {
		return "", false
	}
	return includePath(block.Body.Attributes["path"])
}

// includedBody finds the parsed include at the unit directory or an ancestor.
func includedBody(path, file string, bodies map[string]*hclsyntax.Body) *hclsyntax.Body {
	for dir := filepath.Dir(file); ; dir = filepath.Dir(dir) {
		candidate := filepath.Clean(path)
		if !filepath.IsAbs(path) {
			candidate = filepath.Join(dir, path)
		}
		if included := bodies[candidate]; included != nil {
			return included
		}
		if parent := filepath.Dir(dir); parent == dir || filepath.IsAbs(path) {
			return nil
		}
	}
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
		// A later member replaces an earlier one with the same key, as HCL evaluates the literal. A later
		// key that is not static may name the same member, so no earlier member can be selected: the
		// caller scans the whole object instead.
		for i := len(node.Items) - 1; i >= 0; i-- {
			name, ok := staticKey(node.Items[i].KeyExpr)
			if !ok {
				return nil, false
			}
			if name == key {
				return node.Items[i].ValueExpr, false
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

// indexDiscardsSensitive reports whether source[key] drops a sensitive value that may have been logged.
func (s *sensitivityScanner) indexDiscardsSensitive(index *hclsyntax.IndexExpr, depth int) bool {
	if s.containsSensitive(index.Key, depth) && s.mayLogValue(index.Key, depth) {
		// The selected member is named by a sensitive value, which is not the selected result.
		return true
	}
	key, diags := index.Key.Value(nil)
	if diags.HasErrors() {
		key = cty.DynamicVal
	}
	return s.selectionDiscards(index.Collection, []cty.Value{key}, false, depth)
}

// relativeDiscardsSensitive reports whether source.attr or source[key] steps drop a sensitive value that may
// have been logged.
func (s *sensitivityScanner) relativeDiscardsSensitive(relative *hclsyntax.RelativeTraversalExpr, depth int) bool {
	keys := make([]cty.Value, 0, len(relative.Traversal))
	for _, step := range relative.Traversal {
		switch typed := step.(type) {
		case hcl.TraverseAttr:
			keys = append(keys, cty.StringVal(typed.Name))
		case hcl.TraverseIndex:
			keys = append(keys, typed.Key)
		default:
			keys = append(keys, cty.DynamicVal)
		}
	}
	return s.selectionDiscards(relative.Source, keys, false, depth)
}

// selectionDiscards follows keys into source and reports whether any step drops a member that holds a
// sensitive value that may have been logged. Selecting the sensitive member itself retains it: the sink
// then registers the selected result. A selection that cannot be followed fails closed. wrapped means
// source is the argument of `sensitive`, so every member of it is sensitive.
func (s *sensitivityScanner) selectionDiscards(source hclsyntax.Expression, keys []cty.Value, wrapped bool, depth int) bool {
	if len(keys) == 0 {
		return false
	}
	source = unwrapParentheses(source)
	if call, ok := source.(*hclsyntax.FunctionCallExpr); ok && call.Name == "sensitive" && len(call.Args) == 1 {
		return s.selectionDiscards(call.Args[0], keys, true, depth)
	}
	if (!wrapped && !s.containsSensitive(source, depth)) || !s.mayLogValue(source, depth) {
		return false
	}
	selected, others, ok := selectMember(source, keys[0])
	if !ok {
		return true
	}
	for _, other := range others {
		if (wrapped || s.containsSensitive(other, depth)) && s.mayLogValue(other, depth) {
			return true
		}
	}
	return s.selectionDiscards(selected, keys[1:], wrapped, depth)
}

// selectMember returns the member of an object or tuple literal that key selects, and the expressions of
// every other member. It reports false when the key or the literal's own keys are not statically known.
func selectMember(source hclsyntax.Expression, key cty.Value) (selected hclsyntax.Expression, others []hclsyntax.Expression, ok bool) {
	if !key.IsKnown() || key.IsNull() {
		return nil, nil, false
	}
	switch node := source.(type) {
	case *hclsyntax.ObjectConsExpr:
		if !key.Type().Equals(cty.String) {
			return nil, nil, false
		}
		match := -1
		for i, item := range node.Items {
			name, known := staticKey(item.KeyExpr)
			if !known {
				return nil, nil, false
			}
			if name == key.AsString() {
				match = i
			}
		}
		if match < 0 {
			return nil, nil, false
		}
		for i, item := range node.Items {
			// A repeated key is replaced by the last member, so the earlier ones are dropped too.
			if i != match {
				others = append(others, item.ValueExpr)
			}
		}
		return node.Items[match].ValueExpr, others, true
	case *hclsyntax.TupleConsExpr:
		if !key.Type().Equals(cty.Number) {
			return nil, nil, false
		}
		position, accuracy := key.AsBigFloat().Int64()
		if accuracy != big.Exact || position < 0 || position >= int64(len(node.Exprs)) {
			return nil, nil, false
		}
		for i, expression := range node.Exprs {
			if int64(i) != position {
				others = append(others, expression)
			}
		}
		return node.Exprs[position], others, true
	}
	return nil, nil, false
}
