package template

import (
	"fmt"
	"slices"
	"strconv"
	"text/template"
	"text/template/parse"

	"github.com/cloudposse/atmos/pkg/perf"
)

// DeferCalls rewrites a parsed template so that every simple action calling the template function
// `namespace.method` (for example `atmos.Component`) is emitted verbatim as text when the template
// is executed, instead of being evaluated. The emitted text is itself a template action, so a later
// render pass can evaluate it with a richer context.
//
// Only plain actions are deferred. Actions that declare or assign variables and control structures
// (if, range, with) that call the function are left untouched, because emitting part of them would
// leave the template unbalanced or the declared variables undefined.
//
// A deferred action is emitted in its canonical form (trim markers are not preserved). Field
// references in a deferred action that are rooted at one of resolveRoots (for example `locals`, which
// only exist during the current pass) are replaced by their scalar value from data, so that the
// later pass does not need them. References that cannot be resolved to a scalar are left as is.
//
// It returns the number of actions that were deferred.
func DeferCalls(tree *parse.Tree, namespace, method string, data any, resolveRoots ...string) int {
	defer perf.Track(nil, "template.DeferCalls")()

	if tree == nil || tree.Root == nil {
		return 0
	}

	d := &callDeferrer{namespace: namespace, method: method, data: data, roots: resolveRoots}

	return d.deferInList(tree.Root)
}

// callDeferrer holds the parameters of a single DeferCalls pass.
type callDeferrer struct {
	namespace string
	method    string
	data      any
	roots     []string
}

// deferInList replaces matching actions in list (and in the bodies of control structures) with text nodes.
func (d *callDeferrer) deferInList(list *parse.ListNode) int {
	if list == nil {
		return 0
	}

	count := 0
	for i, node := range list.Nodes {
		switch n := node.(type) {
		case *parse.ActionNode:
			if len(n.Pipe.Decl) == 0 && d.callsFunction(n.Pipe) {
				list.Nodes[i] = &parse.TextNode{NodeType: parse.NodeText, Pos: n.Pos, Text: []byte(d.deferredText(n))}
				count++
			}
		case *parse.IfNode:
			count += d.deferInList(n.List) + d.deferInList(n.ElseList)
		case *parse.RangeNode:
			count += d.deferInList(n.List) + d.deferInList(n.ElseList)
		case *parse.WithNode:
			count += d.deferInList(n.List) + d.deferInList(n.ElseList)
		}
	}

	return count
}

// callsFunction reports whether node contains a call of namespace.method.
func (d *callDeferrer) callsFunction(node parse.Node) bool {
	switch n := node.(type) {
	case *parse.PipeNode:
		return slices.ContainsFunc(n.Cmds, func(cmd *parse.CommandNode) bool { return d.callsFunction(cmd) })
	case *parse.CommandNode:
		return slices.ContainsFunc(n.Args, d.callsFunction)
	case *parse.ChainNode:
		return d.isTargetChain(n) || d.callsFunction(n.Node)
	}

	return false
}

// isTargetChain reports whether the chain is `namespace.method`.
func (d *callDeferrer) isTargetChain(chain *parse.ChainNode) bool {
	id, ok := chain.Node.(*parse.IdentifierNode)

	return ok && id.Ident == d.namespace && len(chain.Field) > 0 && chain.Field[0] == d.method
}

// deferredText renders the action as a template action, with resolvable root references replaced by values.
func (d *callDeferrer) deferredText(action *parse.ActionNode) string {
	pipe := action.Pipe.CopyPipe()
	d.resolveRefs(pipe)

	return "{{" + pipe.String() + "}}"
}

// resolveRefs replaces field references rooted at one of the resolve roots with scalar literals.
func (d *callDeferrer) resolveRefs(pipe *parse.PipeNode) {
	for _, cmd := range pipe.Cmds {
		for i, arg := range cmd.Args {
			switch a := arg.(type) {
			case *parse.FieldNode:
				if literal := d.literalFor(a); literal != nil {
					cmd.Args[i] = literal
				}
			case *parse.PipeNode:
				d.resolveRefs(a)
			case *parse.ChainNode:
				if inner, ok := a.Node.(*parse.PipeNode); ok {
					d.resolveRefs(inner)
				}
			}
		}
	}
}

// literalFor returns a literal node holding the scalar value a field reference resolves to,
// or nil when the reference is not rooted at a resolve root or is not a scalar.
func (d *callDeferrer) literalFor(field *parse.FieldNode) parse.Node {
	if len(field.Ident) == 0 || !slices.Contains(d.roots, field.Ident[0]) {
		return nil
	}

	value, found := LookupFieldPath(d.data, field.Ident)
	if !found {
		return nil
	}

	source, ok := scalarSource(value)
	if !ok {
		return nil
	}

	return parseLiteral(source)
}

// scalarSource returns the template source of a scalar value.
func scalarSource(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		return strconv.Quote(v), true
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return fmt.Sprintf("%v", v), true
	default:
		return "", false
	}
}

// parseLiteral builds a literal node from its template source by parsing it in a tiny template,
// so the node is constructed by the template parser itself.
func parseLiteral(source string) parse.Node {
	tmpl, err := template.New("").Parse("{{" + source + "}}")
	if err != nil || tmpl.Root == nil || len(tmpl.Root.Nodes) != 1 {
		return nil
	}
	action, ok := tmpl.Root.Nodes[0].(*parse.ActionNode)
	if !ok || len(action.Pipe.Cmds) != 1 || len(action.Pipe.Cmds[0].Args) != 1 {
		return nil
	}

	return action.Pipe.Cmds[0].Args[0]
}
