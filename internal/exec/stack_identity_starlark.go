package exec

import (
	"text/template"
	"text/template/parse"

	"github.com/cloudposse/atmos/pkg/naming"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ensureLiteralStackIdentity validates stack identity through the shared naming rules.
func ensureLiteralStackIdentity(name, value string, section map[string]any) error {
	return naming.EnsureLiteralStackIdentity(name, value, section)
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
	return processTmpl(config, &tmplInput{name: name, value: source, data: data, ignoreMissing: ignoreMissing, prepare: func(t *template.Template) {
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
			// Numeric literals cannot contain computed source. Keep them untyped so
			// template functions can accept ideal constants such as 2.0 as integers.
			if _, literal := arg.(*parse.NumberNode); literal {
				continue
			}
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
