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
	"github.com/zclconf/go-cty/cty/convert"
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
	scanner := &sensitivityScanner{locals: localExpressions(body), includes: exposedIncludes(body, file, bodies, includes), file: file, bodies: bodies}
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
	// file and bodies locate the configuration files that read_terragrunt_config reads.
	file   string
	bodies map[string]*hclsyntax.Body
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
			found = typed.Name == "sensitive" || s.readsSensitiveConfig(typed, depth)
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
		// A configuration read from another file carries that file's sensitive values.
		if s.readsSensitiveConfig(call, depth) {
			s.paths = append(s.paths, slices.Clone(prefix))
			return
		}
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
	if len(traversal) == 1 && traversal.RootName() == localName {
		// A bare local copies every local of the file, as include.<alias>.locals copies the included file's.
		return wholeLocals(exposedInclude{locals: s.locals, known: true}), true
	}
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
			return wholeLocals(exposed), true
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

// wholeLocals stands for include.<alias>.locals itself, which copies every local of the included file. A
// file that was not found cannot be inspected, so it reads as sensitive as a whole.
func wholeLocals(exposed exposedInclude) sensitivityReference {
	if !exposed.known {
		return sensitivityReference{unknownExpression, nil, nil}
	}
	names := make([]string, 0, len(exposed.locals))
	for name := range exposed.locals {
		names = append(names, name)
	}
	slices.Sort(names)
	object := &hclsyntax.ObjectConsExpr{}
	for _, name := range names {
		object.Items = append(object.Items, hclsyntax.ObjectConsItem{
			KeyExpr:   &hclsyntax.LiteralValueExpr{Val: cty.StringVal(name)},
			ValueExpr: exposed.locals[name],
		})
	}
	return sensitivityReference{object, exposed.locals, nil}
}

// readsSensitiveConfig reports whether call is a read_terragrunt_config of a file whose inputs or locals can
// hold a sensitive value. The result of the call exposes both, and the scan does not follow which of them is
// selected, so the whole result reads as sensitive. A file that cannot be inspected, a path that is not
// static, and a file that includes another (whose inputs it may inherit) fail closed.
func (s *sensitivityScanner) readsSensitiveConfig(call *hclsyntax.FunctionCallExpr, depth int) bool {
	if call.Name != readConfigFunction {
		return false
	}
	if depth >= maxLocalDepth || len(call.Args) == 0 {
		return true
	}
	path, parentSearch, ok := includePath(&hclsyntax.Attribute{Expr: call.Args[0]})
	if !ok {
		return true
	}
	target, targetFile := readFile(path, parentSearch, s.file, s.bodies)
	if target == nil && !parentSearch && len(call.Args) > 1 {
		// A missing literal file yields the default argument, which the caller scans like any other argument.
		return false
	}
	if target == nil || slices.ContainsFunc(target.Blocks, func(block *hclsyntax.Block) bool { return block.Type == "include" }) {
		return true
	}
	reader := &sensitivityScanner{locals: localExpressions(target), file: targetFile, bodies: s.bodies}
	expressions := make([]hclsyntax.Expression, 0, len(reader.locals)+1)
	if attribute := target.Attributes[inputsName]; attribute != nil {
		expressions = append(expressions, attribute.Expr)
	}
	for _, expression := range reader.locals {
		expressions = append(expressions, expression)
	}
	return slices.ContainsFunc(expressions, func(expression hclsyntax.Expression) bool {
		return reader.containsSensitive(expression, depth+1)
	})
}

// exposedIncludes associates an include alias with the parsed file's locals and inputs.
func exposedIncludes(body *hclsyntax.Body, file string, bodies map[string]*hclsyntax.Body, evaluated config.IncludeConfigsMap) map[string]exposedInclude {
	result := make(map[string]exposedInclude)
	for _, block := range body.Blocks {
		if block.Type != "include" || len(block.Labels) == 0 {
			continue
		}
		path, parentSearch, ok := exposedIncludePath(block, evaluated)
		if !ok {
			continue
		}
		included, _ := includedFile(path, parentSearch, file, bodies)
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

// exposedIncludePath reads a statically exposed include's path and whether it comes from a parent-folder
// search. An evaluated path is already resolved, so it never needs the search.
func exposedIncludePath(block *hclsyntax.Block, evaluated config.IncludeConfigsMap) (path string, parentSearch, ok bool) {
	if evaluated != nil {
		include, found := evaluated[block.Labels[0]]
		return include.Path, false, found && include.Expose != nil && *include.Expose
	}
	expose, ok := block.Body.Attributes["expose"]
	if !ok {
		return "", false, false
	}
	value, diagnostics := expose.Expr.Value(nil)
	if diagnostics.HasErrors() || !value.IsKnown() || value.IsNull() || !value.Type().Equals(cty.Bool) || !value.True() {
		return "", false, false
	}
	return includePath(block.Body.Attributes["path"])
}

// includedFile finds the parsed file at path, searching the directory of file and its ancestors, and returns
// the filename the body was found under, which is the base for any configuration that file reads in turn. A
// parent-folder search (find_in_parent_folders) starts in the parent of that directory, as Terragrunt does,
// so a file of the same name beside the caller is never selected for it.
func includedFile(path string, parentSearch bool, file string, bodies map[string]*hclsyntax.Body) (*hclsyntax.Body, string) {
	start := filepath.Dir(file)
	if parentSearch {
		start = filepath.Dir(start)
	}
	for dir := start; ; dir = filepath.Dir(dir) {
		candidate := filepath.Clean(path)
		if !filepath.IsAbs(path) {
			candidate = filepath.Join(dir, path)
		}
		if included := bodies[candidate]; included != nil {
			return included, candidate
		}
		if parent := filepath.Dir(dir); parent == dir || filepath.IsAbs(path) {
			return nil, ""
		}
	}
}

// readFile finds the file a read_terragrunt_config call reads. A parent-folder search walks up the tree, but
// a literal path is relative to the file that names it and is not searched for in ancestors: when that file
// is missing Terragrunt uses the call's default (or fails), so a file of the same name above must not be
// taken for it.
func readFile(path string, parentSearch bool, file string, bodies map[string]*hclsyntax.Body) (*hclsyntax.Body, string) {
	if parentSearch {
		return includedFile(path, parentSearch, file, bodies)
	}
	candidate := filepath.Clean(path)
	if !filepath.IsAbs(path) {
		candidate = filepath.Join(filepath.Dir(file), path)
	}
	if included := bodies[candidate]; included != nil {
		return included, candidate
	}
	return nil, ""
}

// includePath reads the static file name of a path attribute. parentSearch reports that the name is the
// argument of find_in_parent_folders, which looks above the current directory, rather than a literal path
// relative to the file that names it.
func includePath(attribute *hclsyntax.Attribute) (path string, parentSearch, ok bool) {
	if attribute == nil {
		return "", false, false
	}
	expression := attribute.Expr
	if call, isCall := expression.(*hclsyntax.FunctionCallExpr); isCall && call.Name == "find_in_parent_folders" && len(call.Args) > 0 {
		expression = call.Args[0]
		parentSearch = true
	}
	value, diagnostics := expression.Value(nil)
	if diagnostics.HasErrors() || !value.IsKnown() || value.IsNull() || !value.Type().Equals(cty.String) {
		return "", false, false
	}
	return value.AsString(), parentSearch, true
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
	return s.selectionDiscards(relative.Source, traverserKeys(relative.Traversal), false, depth)
}

// splatDiscardsSensitive reports whether source[*].attr drops a sensitive value that may have been logged.
// A splat selects the same steps from every element of a tuple, or from a single value that is not a
// collection, so each element is followed like a selection of its own. A source or a projection that cannot
// be followed fails closed, while a projection that only keeps the sensitive member retains it.
func (s *sensitivityScanner) splatDiscardsSensitive(splat *hclsyntax.SplatExpr, depth int) bool {
	keys, ok := splatKeys(splat.Each)
	if !ok {
		return s.containsSensitive(splat.Source, depth) && s.mayLogValue(splat.Source, depth)
	}
	source, wrapped := splat.Source, false
	if call, isCall := unwrapParentheses(source).(*hclsyntax.FunctionCallExpr); isCall && call.Name == "sensitive" && len(call.Args) == 1 {
		source, wrapped = call.Args[0], true
	}
	if tuple, isTuple := unwrapParentheses(source).(*hclsyntax.TupleConsExpr); isTuple {
		for _, element := range tuple.Exprs {
			if s.selectionDiscards(element, keys, wrapped, depth) {
				return true
			}
		}
		return false
	}
	return s.selectionDiscards(source, keys, wrapped, depth)
}

// splatKeys returns the steps a splat's projection applies to each element. The projection is written
// against the anonymous element, so it is a chain of attribute and index steps that ends at it.
func splatKeys(projection hclsyntax.Expression) ([]cty.Value, bool) {
	switch node := projection.(type) {
	case *hclsyntax.AnonSymbolExpr:
		return nil, true
	case *hclsyntax.RelativeTraversalExpr:
		keys, ok := splatKeys(node.Source)
		return append(keys, traverserKeys(node.Traversal)...), ok
	case *hclsyntax.IndexExpr:
		keys, ok := splatKeys(node.Collection)
		key, diags := node.Key.Value(nil)
		if diags.HasErrors() {
			key = cty.DynamicVal
		}
		return append(keys, key), ok
	}
	return nil, false
}

// traverserKeys lists the keys a traversal selects; a step that is not an attribute or an index is unknown.
func traverserKeys(traversal hcl.Traversal) []cty.Value {
	keys := make([]cty.Value, 0, len(traversal))
	for _, step := range traversal {
		switch typed := step.(type) {
		case hcl.TraverseAttr:
			keys = append(keys, cty.StringVal(typed.Name))
		case hcl.TraverseIndex:
			keys = append(keys, typed.Key)
		default:
			keys = append(keys, cty.DynamicVal)
		}
	}
	return keys
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
		// HCL names an object member by the string its key converts to, so 1 selects "1".
		key, ok = convertedKey(key, cty.String)
		if !ok {
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
		// HCL names a tuple element by the number its key converts to, so "1" selects element 1.
		key, ok = convertedKey(key, cty.Number)
		if !ok {
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

// convertedKey converts a selection key to the type HCL indexes the collection with. A key that does not
// convert, such as "x" for a tuple, is reported as unusable so the selection fails closed.
func convertedKey(key cty.Value, target cty.Type) (cty.Value, bool) {
	converted, err := convert.Convert(key, target)
	if err != nil || !converted.IsKnown() || converted.IsNull() {
		return cty.NilVal, false
	}
	return converted, true
}
