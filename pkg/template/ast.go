// Package template provides utilities for Go template AST inspection and analysis.
// This package enables extraction of field references from templates, which is
// useful for dependency resolution (e.g., locals referencing other locals).
package template

import (
	"reflect"
	"strings"
	"text/template"
	"text/template/parse"

	"github.com/cloudposse/atmos/pkg/perf"
)

// templateOpenDelim is the default Go-template opening delimiter, used as a cheap pre-check
// before parsing: a string containing none of it cannot possibly hold a template action.
const templateOpenDelim = "{{"

// FieldRef represents a reference to a field in a template (e.g., .locals.foo).
type FieldRef struct {
	Path []string // e.g., ["locals", "foo"] for .locals.foo
}

// String returns the dot-separated path of the field reference.
func (f FieldRef) String() string {
	defer perf.Track(nil, "template.FieldRef.String")()

	return strings.Join(f.Path, ".")
}

// ExtractFieldRefs parses a Go template string and extracts all field references.
// Handles complex expressions: conditionals, pipes, range, with blocks, nested templates.
// Returns nil if the string is not a valid template or contains no field references.
func ExtractFieldRefs(templateStr string) ([]FieldRef, error) {
	defer perf.Track(nil, "template.ExtractFieldRefs")()

	// Quick check - if no template delimiters, no refs possible.
	if !strings.Contains(templateStr, templateOpenDelim) {
		return nil, nil
	}

	tmpl, err := template.New("").Parse(templateStr)
	if err != nil {
		return nil, err
	}

	tree := tmpl.Tree
	if tree == nil || tree.Root == nil {
		return nil, nil
	}

	var refs []FieldRef
	seen := make(map[string]bool)

	walkAST(tree.Root, func(node parse.Node) {
		if field, ok := node.(*parse.FieldNode); ok {
			key := fieldKey(field.Ident)
			if !seen[key] {
				refs = append(refs, FieldRef{Path: field.Ident})
				seen[key] = true
			}
		}
	})

	return refs, nil
}

// ExtractFieldRefsByPrefix extracts field references that start with a specific prefix.
// For example, ExtractFieldRefsByPrefix(tmpl, "locals") returns all .locals.X references.
// Returns the second-level identifiers (e.g., "foo" for .locals.foo).
func ExtractFieldRefsByPrefix(templateStr string, prefix string) ([]string, error) {
	defer perf.Track(nil, "template.ExtractFieldRefsByPrefix")()

	refs, err := ExtractFieldRefs(templateStr)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var result []string
	for _, ref := range refs {
		if len(ref.Path) >= 2 && ref.Path[0] == prefix {
			name := ref.Path[1]
			if !seen[name] {
				result = append(result, name)
				seen[name] = true
			}
		}
	}
	return result, nil
}

// walkAST traverses all nodes in a template AST, calling fn for each node.
// This handles all Go template node types including conditionals, ranges,
// with blocks, and nested templates.
func walkAST(node parse.Node, fn func(parse.Node)) {
	if node == nil {
		return
	}

	fn(node)

	switch n := node.(type) {
	case *parse.ListNode:
		walkListNode(n, fn)

	case *parse.ActionNode:
		walkAST(n.Pipe, fn)

	case *parse.PipeNode:
		walkPipeNode(n, fn)

	case *parse.CommandNode:
		walkCommandNode(n, fn)

	case *parse.IfNode:
		walkBranchNode(n.Pipe, n.List, n.ElseList, fn)

	case *parse.RangeNode:
		walkBranchNode(n.Pipe, n.List, n.ElseList, fn)

	case *parse.WithNode:
		walkBranchNode(n.Pipe, n.List, n.ElseList, fn)

	case *parse.TemplateNode:
		walkAST(n.Pipe, fn)

	case *parse.ChainNode:
		// A chain is a field access on an expression, e.g. `(ds "cfg").name`
		// or `atmos.Component "x" "y"`; the expression itself may contain
		// function calls and field references.
		walkAST(n.Node, fn)
	}
}

// walkListNode traverses a ListNode and processes its children.
func walkListNode(n *parse.ListNode, fn func(parse.Node)) {
	if n == nil {
		return
	}
	for _, child := range n.Nodes {
		walkAST(child, fn)
	}
}

// walkPipeNode traverses a PipeNode and processes commands and declarations.
func walkPipeNode(n *parse.PipeNode, fn func(parse.Node)) {
	if n == nil {
		return
	}
	for _, cmd := range n.Cmds {
		walkAST(cmd, fn)
	}
	for _, decl := range n.Decl {
		walkAST(decl, fn)
	}
}

// walkCommandNode traverses a CommandNode and processes arguments.
func walkCommandNode(n *parse.CommandNode, fn func(parse.Node)) {
	if n == nil {
		return
	}
	for _, arg := range n.Args {
		walkAST(arg, fn)
	}
}

// walkBranchNode traverses branch nodes (if/range/with) with pipe, list, and else-list.
func walkBranchNode(pipe *parse.PipeNode, list, elseList *parse.ListNode, fn func(parse.Node)) {
	walkAST(pipe, fn)
	walkAST(list, fn)
	walkAST(elseList, fn)
}

// fieldKey creates a unique key from a field path for deduplication.
func fieldKey(ident []string) string {
	return strings.Join(ident, ".")
}

// WalkNodes calls fn for every node in every template associated with tmpl
// (including tmpl itself, and any template declared inside it with `define`),
// using the same traversal rules as UsesFunctions. It exists so packages
// outside this one can run their own analysis passes (for example, a
// deprecation lint) over a parsed template without duplicating the AST
// traversal.
func WalkNodes(tmpl *template.Template, fn func(parse.Node)) {
	defer perf.Track(nil, "template.WalkNodes")()

	if tmpl == nil {
		return
	}
	for _, t := range tmpl.Templates() {
		if t.Tree == nil || t.Root == nil {
			continue
		}
		walkAST(t.Root, fn)
	}
}

// UsesFunctions reports whether the parsed template (or any template
// associated with it, e.g. one declared with `define`) calls any of the
// named functions, or any of the named `identifier.Method` chains
// (e.g. "atmos.GomplateDatasource"). Function calls appear in the AST as
// IdentifierNodes; a method call on a function's result appears as a
// ChainNode whose head is that IdentifierNode. Only names present in the
// maps are matched, so callers can pass an empty map for either set.
func UsesFunctions(tmpl *template.Template, funcNames map[string]struct{}, methodChains map[string]struct{}) bool {
	defer perf.Track(nil, "template.UsesFunctions")()

	if tmpl == nil {
		return false
	}

	found := false
	for _, t := range tmpl.Templates() {
		if found || t.Tree == nil || t.Root == nil {
			continue
		}
		walkAST(t.Root, func(node parse.Node) {
			if !found && matchesFunction(node, funcNames, methodChains) {
				found = true
			}
		})
	}

	return found
}

// matchesFunction reports whether node is a call to one of funcNames or a
// method chain listed in methodChains.
func matchesFunction(node parse.Node, funcNames map[string]struct{}, methodChains map[string]struct{}) bool {
	switch n := node.(type) {
	case *parse.IdentifierNode:
		_, ok := funcNames[n.Ident]
		return ok
	case *parse.ChainNode:
		ident, ok := n.Node.(*parse.IdentifierNode)
		if !ok || len(n.Field) == 0 {
			return false
		}
		_, ok = methodChains[ident.Ident+"."+n.Field[0]]
		return ok
	default:
		return false
	}
}

// HasTemplateActions checks if a string contains Go template actions.
// This is a more robust version that uses AST parsing instead of simple string matching.
func HasTemplateActions(str string) (bool, error) {
	defer perf.Track(nil, "template.HasTemplateActions")()

	// Quick check - if no template delimiters, no actions possible.
	if !strings.Contains(str, templateOpenDelim) {
		return false, nil
	}

	tmpl, err := template.New("").Parse(str)
	if err != nil {
		return false, err
	}

	tree := tmpl.Tree
	if tree == nil || tree.Root == nil {
		return false, nil
	}

	hasActions := false
	walkAST(tree.Root, func(node parse.Node) {
		switch node.(type) {
		case *parse.ActionNode, *parse.IfNode, *parse.RangeNode, *parse.WithNode:
			hasActions = true
		}
	})

	return hasActions, nil
}

// ExtractAllFieldRefsByPrefix extracts all field references that start with a specific prefix,
// returning the full remaining path after the prefix.
// For example, ExtractAllFieldRefsByPrefix(tmpl, "locals") for .locals.config.nested
// returns ["config.nested"].
func ExtractAllFieldRefsByPrefix(templateStr string, prefix string) ([]string, error) {
	defer perf.Track(nil, "template.ExtractAllFieldRefsByPrefix")()

	refs, err := ExtractFieldRefs(templateStr)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var result []string
	for _, ref := range refs {
		if len(ref.Path) >= 2 && ref.Path[0] == prefix {
			// Join all path elements after the prefix.
			fullPath := strings.Join(ref.Path[1:], ".")
			if !seen[fullPath] {
				result = append(result, fullPath)
				seen[fullPath] = true
			}
		}
	}
	return result, nil
}

// ExtractPlainFieldRef returns the field reference when the template is exactly
// one plain field action, such as "{{ .locals.default_tags }}".
func ExtractPlainFieldRef(templateStr string) (FieldRef, bool, error) {
	defer perf.Track(nil, "template.ExtractPlainFieldRef")()

	if !strings.Contains(templateStr, templateOpenDelim) {
		return FieldRef{}, false, nil
	}

	tmpl, err := template.New("").Parse(templateStr)
	if err != nil {
		return FieldRef{}, false, nil
	}
	action, ok := singlePlainAction(tmpl)
	if !ok {
		return FieldRef{}, false, nil
	}

	ref, ok := fieldRefFromAction(action)
	return ref, ok, nil
}

// Returns the template's only action node when the template consists of
// exactly one action surrounded by nothing but whitespace.
func singlePlainAction(tmpl *template.Template) (*parse.ActionNode, bool) {
	if tmpl.Tree == nil || tmpl.Root == nil {
		return nil, false
	}

	var action *parse.ActionNode
	for _, node := range tmpl.Root.Nodes {
		if text, ok := node.(*parse.TextNode); ok {
			if strings.TrimSpace(string(text.Text)) != "" {
				return nil, false
			}
			continue
		}

		next, ok := node.(*parse.ActionNode)
		if !ok || action != nil {
			return nil, false
		}
		action = next
	}
	return action, action != nil
}

// Extracts the field reference from an action that is a single, undecorated
// field access such as .a.b with no pipeline or arguments.
func fieldRefFromAction(action *parse.ActionNode) (FieldRef, bool) {
	if action.Pipe == nil || len(action.Pipe.Decl) != 0 || len(action.Pipe.Cmds) != 1 {
		return FieldRef{}, false
	}

	cmd := action.Pipe.Cmds[0]
	if cmd == nil || len(cmd.Args) != 1 {
		return FieldRef{}, false
	}

	field, ok := cmd.Args[0].(*parse.FieldNode)
	if !ok || len(field.Ident) == 0 {
		return FieldRef{}, false
	}

	return FieldRef{Path: field.Ident}, true
}

// HasDynamicScope reports whether a template contains a `range` or `with` action, either of
// which rebinds "." to something other than the template's root data for the body it encloses.
// Field references found inside such a body (e.g. `.name` inside
// `{{ range .vars.list }}{{ .name }}{{ end }}`) are NOT relative to the template's root, so a
// caller using ExtractFieldRefs to statically determine which root-level fields a template
// touches (see pkg/list/column.RequiredSections) must treat any template containing dynamic
// scope as unresolvable rather than misattributing those inner references to the root.
func HasDynamicScope(templateStr string) (bool, error) {
	defer perf.Track(nil, "template.HasDynamicScope")()

	if !strings.Contains(templateStr, templateOpenDelim) {
		return false, nil
	}

	tmpl, err := template.New("").Parse(templateStr)
	if err != nil {
		return false, err
	}

	tree := tmpl.Tree
	if tree == nil || tree.Root == nil {
		return false, nil
	}

	dynamic := false
	walkAST(tree.Root, func(node parse.Node) {
		switch node.(type) {
		case *parse.RangeNode, *parse.WithNode:
			dynamic = true
		}
	})

	return dynamic, nil
}

// HasRootReference reports whether a template contains a bare "." action (*parse.DotNode) --
// e.g. "{{ . }}" or "{{ printf "%v" . }}" -- which accesses the entire root data value rather
// than a specific named field. ExtractFieldRefs only records *parse.FieldNode matches, so a
// template consisting solely of "{{ . }}" yields zero field references even though it exposes
// every field of the root, including ones no FieldNode ever names. Callers like
// pkg/list/column.RequiredSections that statically enumerate which root-level fields a template
// touches must treat any such reference as unresolvable rather than silently recording it as
// touching nothing.
func HasRootReference(templateStr string) (bool, error) {
	defer perf.Track(nil, "template.HasRootReference")()

	if !strings.Contains(templateStr, templateOpenDelim) {
		return false, nil
	}

	tmpl, err := template.New("").Parse(templateStr)
	if err != nil {
		return false, err
	}

	tree := tmpl.Tree
	if tree == nil || tree.Root == nil {
		return false, nil
	}

	root := false
	walkAST(tree.Root, func(node parse.Node) {
		if _, ok := node.(*parse.DotNode); ok {
			root = true
		}
	})

	return root, nil
}

// LookupFieldPath resolves a field path against map-like data.
func LookupFieldPath(data any, path []string) (any, bool) {
	defer perf.Track(nil, "template.LookupFieldPath")()

	current := data
	for _, part := range path {
		next, ok := lookupMapValue(current, part)
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

// Reads key from a map[string]any or any string-keyed map.
func lookupMapValue(data any, key string) (any, bool) {
	if data == nil {
		return nil, false
	}
	if typed, ok := data.(map[string]any); ok {
		val, found := typed[key]
		return val, found
	}

	value := reflect.ValueOf(data)
	if value.Kind() == reflect.Map && value.Type().Key().Kind() == reflect.String {
		mapValue := value.MapIndex(reflect.ValueOf(key))
		if !mapValue.IsValid() {
			return nil, false
		}
		return mapValue.Interface(), true
	}

	return nil, false
}
