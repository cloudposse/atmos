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

// FieldRef represents a reference to a field in a template (e.g., .locals.foo).
type FieldRef struct {
	Path []string // e.g., ["locals", "foo"] for .locals.foo
}

// String returns the dot-separated path of the field reference.
func (f FieldRef) String() string {
	return strings.Join(f.Path, ".")
}

// ExtractFieldRefs parses a Go template string and extracts all field references.
// Handles complex expressions: conditionals, pipes, range, with blocks, nested templates.
// Returns nil if the string is not a valid template or contains no field references.
func ExtractFieldRefs(templateStr string) ([]FieldRef, error) {
	defer perf.Track(nil, "template.ExtractFieldRefs")()

	// Quick check - if no template delimiters, no refs possible.
	if !strings.Contains(templateStr, "{{") {
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

// HasTemplateActions checks if a string contains Go template actions.
// This is a more robust version that uses AST parsing instead of simple string matching.
func HasTemplateActions(str string) (bool, error) {
	defer perf.Track(nil, "template.HasTemplateActions")()

	// Quick check - if no template delimiters, no actions possible.
	if !strings.Contains(str, "{{") {
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

	if !strings.Contains(templateStr, "{{") {
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
