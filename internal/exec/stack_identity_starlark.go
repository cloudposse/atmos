package exec

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"text/template/parse"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/function/starlarksource"
	"github.com/cloudposse/atmos/pkg/schema"
)

// identitySections lists the component sections that stacks.name_template and stacks.name_pattern read,
// in the order they are searched when reporting which field made the stack identity computed.
var identitySections = []string{"vars", "settings", "metadata", "env"}

// ensureLiteralStackIdentity rejects a stack name that was derived from a !starlark value.
// A !starlark value only has a result after the stack is known, so it cannot decide the
// stack's own identity. Silently using the encoded source as the name would produce stacks,
// workspaces, and files named after the program text. The error names the manifest and, when
// it can be found, the field whose value the name came from.
func ensureLiteralStackIdentity(stackFileName, name string, section map[string]any) error {
	if !containsStarlark(name) && !strings.Contains(name, starlarksource.Tag+"\n") {
		return nil
	}
	builder := errUtils.Build(errUtils.ErrStarlarkStackIdentity).
		WithContext("manifest", stackFileName)
	explanation := fmt.Sprintf("The stack name for manifest `%s` is derived from a !starlark value.", stackFileName)
	if field := starlarkIdentityField(section, name); field != "" {
		builder = builder.WithContext("field", field)
		explanation = fmt.Sprintf("The stack name for manifest `%s` is derived from `%s`, which is a !starlark value.", stackFileName, field)
	}
	return builder.
		WithExplanation(explanation).
		WithHint("Set a literal value for the fields used by stacks.name_template or stacks.name_pattern.").
		Err()
}

// starlarkIdentityField returns the dotted path (for example "vars.stage") of the !starlark
// value that the stack name was built from, or "" when no field can be tied to the name.
// Only a value whose encoded source appears in the name qualifies, so an unrelated
// !starlark field in the same section is never blamed.
func starlarkIdentityField(section map[string]any, name string) string {
	for _, sectionName := range identitySections {
		values, ok := section[sectionName].(map[string]any)
		if !ok {
			continue
		}
		if path := findStarlarkPath(sectionName, values, name); path != "" {
			return path
		}
	}
	return ""
}

func findStarlarkPath(prefix string, value any, name string) string {
	switch value := value.(type) {
	case string:
		if containsStarlark(value) && strings.Contains(name, value) {
			return prefix
		}
	case map[string]any:
		for _, key := range slices.Sorted(maps.Keys(value)) {
			if path := findStarlarkPath(prefix+"."+key, value[key], name); path != "" {
				return path
			}
		}
	case []any:
		for i, item := range value {
			if path := findStarlarkPath(prefix+"."+strconv.Itoa(i), item, name); path != "" {
				return path
			}
		}
	}
	return ""
}

// processStackNameTemplate guards the values actually evaluated by a naming template. Checking
// the rendered string alone loses provenance when printf, hashing, or slicing transforms source.
func processStackNameTemplate(config *schema.AtmosConfiguration, name, source string, data any, ignoreMissing bool) (string, error) {
	section, _ := data.(map[string]any)
	guard := func(value any) (any, error) {
		if text, ok := value.(string); ok {
			return value, ensureLiteralStackIdentity(name, text, section)
		}
		return value, nil
	}
	guardArgument := func(value any) (any, error) {
		return value, checkIdentityArgument(name, value, section)
	}
	return processTmpl(config, name, source, data, templateProcessOptions{ignoreMissing: ignoreMissing, prepare: func(t *template.Template) {
		t.Funcs(template.FuncMap{identityValueGuard: guard, identityArgumentGuard: guardArgument})
		for _, defined := range t.Templates() {
			guardIdentityNodes(defined.Root)
		}
	}})
}

const (
	identityValueGuard    = "__atmos_literal_identity_value"
	identityArgumentGuard = "__atmos_literal_identity_argument"
)

// checkIdentityArgument checks containers when a function consumes their contents, so formatting
// a whole map cannot hide computed values. Lookups check only their selected result instead.
func checkIdentityArgument(name string, value any, section map[string]any) error {
	switch value := value.(type) {
	case string:
		return ensureLiteralStackIdentity(name, value, section)
	case map[string]any:
		for _, child := range value {
			if err := checkIdentityArgument(name, child, section); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range value {
			if err := checkIdentityArgument(name, child, section); err != nil {
				return err
			}
		}
	}
	return nil
}

// guardIdentityNodes instruments evaluated expressions without evaluating unchosen branches or
// rejecting unrelated computed fields in the template's input map.
func guardIdentityNodes(node parse.Node) {
	switch node := node.(type) {
	case *parse.ListNode:
		if node != nil {
			for _, child := range node.Nodes {
				guardIdentityNodes(child)
			}
		}
	case *parse.ActionNode:
		guardIdentityPipe(node.Pipe)
		if len(node.Pipe.Decl) == 0 {
			node.Pipe.Cmds = append(node.Pipe.Cmds, identityGuardCommand(identityArgumentGuard))
		}
	case *parse.IfNode:
		guardIdentityBranch(&node.BranchNode)
	case *parse.WithNode:
		guardIdentityBranch(&node.BranchNode)
	case *parse.RangeNode:
		guardIdentityBranch(&node.BranchNode)
	case *parse.TemplateNode:
		guardIdentityPipe(node.Pipe)
	}
}

func guardIdentityBranch(node *parse.BranchNode) {
	guardIdentityPipe(node.Pipe)
	guardIdentityNodes(node.List)
	guardIdentityNodes(node.ElseList)
}

// identitySelectsValue identifies functions that navigate a container without consuming every
// value. Their result is still guarded before it reaches another function or a branch condition.
func identitySelectsValue(command *parse.CommandNode) bool {
	if id, ok := command.Args[0].(*parse.IdentifierNode); ok {
		switch id.Ident {
		case "index", "get", "dig", "hasKey", "len", "keys", "pick", "omit", "pluck", "default", "coalesce", "empty", "and", "or":
			return true
		}
	}
	return false
}

func guardIdentityPipe(pipe *parse.PipeNode) {
	if pipe == nil {
		return
	}
	commands := make([]*parse.CommandNode, 0, 2*len(pipe.Cmds))
	for i, command := range pipe.Cmds {
		guard := identityArgumentGuard
		if identitySelectsValue(command) {
			guard = identityValueGuard
		}
		if i > 0 {
			commands = append(commands, identityGuardCommand(guard))
		}
		for j, arg := range command.Args {
			if nested, ok := arg.(*parse.PipeNode); ok {
				guardIdentityPipe(nested)
			}
			if j > 0 {
				command.Args[j] = &parse.PipeNode{NodeType: parse.NodePipe, Cmds: []*parse.CommandNode{
					{NodeType: parse.NodeCommand, Args: []parse.Node{parse.NewIdentifier(guard), arg}},
				}}
			}
		}
		commands = append(commands, command, identityGuardCommand(identityValueGuard))
	}
	pipe.Cmds = commands
}

func identityGuardCommand(name string) *parse.CommandNode {
	return &parse.CommandNode{NodeType: parse.NodeCommand, Args: []parse.Node{parse.NewIdentifier(name)}}
}
