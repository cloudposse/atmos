package deferred

import (
	"slices"

	"go.starlark.net/syntax"

	"github.com/cloudposse/atmos/pkg/function/starlarksource"
	"github.com/cloudposse/atmos/pkg/perf"
)

// starlarkSections are the ctx fields that expose the component's configuration sections.
var starlarkSections = []string{"vars", "metadata", "settings", "env", "locals"}

// starlarkIdentityFields are the ctx fields Atmos supplies itself. Reading them depends on no
// configuration value.
var starlarkIdentityFields = []string{"stack", "component", "component_type"}

// starlarkMapMethods are the methods of a configuration mapping. Only `get` with a literal key
// names a single entry; the others enumerate the mapping.
var starlarkMapMethods = []string{"get", "items", "keys", "values"}

// starlarkReferenceOptions parse a !starlark body the way the interpreter does.
var starlarkReferenceOptions = &syntax.FileOptions{TopLevelControl: true, While: true, Set: true, Recursion: true}

// StarlarkReferences returns the configuration fields a !starlark value reads through ctx, so a
// selective command can evaluate only those fields instead of the whole component. Recognized reads
// are ctx.<section>.<name> and ctx.<section>["name"] (chains of either form, and
// ctx.<section>.get("name")), where the section is vars, metadata, settings, env, or locals, and the
// identity fields ctx.stack, ctx.component, and ctx.component_type.
//
// It reports false when the value reads configuration in a way static analysis cannot bound:
// a whole section (dict(ctx.vars), json.encode(ctx.vars), iteration, an alias such as v = ctx.vars),
// a computed key (ctx.vars[name]), items(), keys(), values(), a get() whose key is not a string
// literal, ctx itself used as a value, an unknown ctx field, or source that does not parse. The
// caller then evaluates the full component, as it does for any construct it cannot analyze.
func StarlarkReferences(value string) ([][]string, bool) {
	defer perf.Track(nil, "deferred.StarlarkReferences")()

	source := starlarksource.Decode(value)
	file, err := starlarkReferenceOptions.Parse("!starlark", []byte(source.Code), 0)
	if err != nil {
		return nil, false
	}
	var (
		paths   [][]string
		bounded = true
		stack   []syntax.Node
	)
	syntax.Walk(file, func(node syntax.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		stack = append(stack, node)
		if ident, ok := node.(*syntax.Ident); ok && ident.Name == "ctx" && bounded {
			path, known, ok := ctxReference(stack)
			switch {
			case !ok:
				bounded = false
			case known:
				paths = append(paths, path)
			}
		}
		return bounded
	})
	if !bounded {
		return nil, false
	}
	return paths, true
}

// Classifies one use of the ctx identifier. The stack argument ends with the identifier and holds
// its ancestors. The ok result is false when the use cannot be bounded, and known is false when it
// needs no configuration field (an identity field).
func ctxReference(stack []syntax.Node) (path []string, known, ok bool) {
	names := make([]string, 0, 4)
	depth := len(stack) - 1
	for depth > 0 {
		parent := stack[depth-1]
		name, isName := chainName(parent, stack[depth])
		if !isName {
			break
		}
		names = append(names, name)
		depth--
	}
	if len(names) == 0 {
		return nil, false, false
	}
	if slices.Contains(starlarkIdentityFields, names[0]) {
		return nil, false, true
	}
	if !slices.Contains(starlarkSections, names[0]) || len(names) < 2 {
		return nil, false, false
	}
	return sectionPath(names, stack, depth)
}

// chainName returns the field a DotExpr or literal-string IndexExpr selects from child.
func chainName(parent, child syntax.Node) (string, bool) {
	switch expr := parent.(type) {
	case *syntax.DotExpr:
		if expr.X == child {
			return expr.Name.Name, true
		}
	case *syntax.IndexExpr:
		if expr.X != child {
			return "", false
		}
		if literal, ok := expr.Y.(*syntax.Literal); ok && literal.Token == syntax.STRING {
			if text, ok := literal.Value.(string); ok {
				return text, true
			}
		}
	}
	return "", false
}

// Turns the field names read from a section into a configuration path. Names are in the order
// they appear after ctx, and depth is the stack index of the outermost chain expression.
func sectionPath(names []string, stack []syntax.Node, depth int) ([]string, bool, bool) {
	for i := 1; i < len(names); i++ {
		if !slices.Contains(starlarkMapMethods, names[i]) {
			continue
		}
		// A method name selects a configuration entry only as a map method; anything but
		// get("literal") enumerates the mapping.
		if names[i] != "get" || i != len(names)-1 {
			return nil, false, false
		}
		key, ok := literalGetKey(stack, depth)
		if !ok {
			return nil, false, false
		}
		return append(slices.Clone(names[:i]), key), true, true
	}
	return slices.Clone(names), true, true
}

// literalGetKey returns the string literal passed as the first positional argument of the call
// that applies the outermost chain expression, as in ctx.vars.get("name", default).
func literalGetKey(stack []syntax.Node, depth int) (string, bool) {
	if depth < 1 {
		return "", false
	}
	call, ok := stack[depth-1].(*syntax.CallExpr)
	if !ok || call.Fn != stack[depth] || len(call.Args) == 0 {
		return "", false
	}
	literal, ok := call.Args[0].(*syntax.Literal)
	if !ok || literal.Token != syntax.STRING {
		return "", false
	}
	key, ok := literal.Value.(string)
	return key, ok
}
