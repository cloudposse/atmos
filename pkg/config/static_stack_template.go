package config

import (
	"text/template/parse"

	"github.com/cloudposse/atmos/pkg/function/literalsource"
	fntag "github.com/cloudposse/atmos/pkg/function/tag"
	"github.com/cloudposse/atmos/pkg/function/yamlsource"
	"github.com/cloudposse/atmos/pkg/template/delimiters"
)

// staticNameInputs inspects template reads before execution. Checking only the
// rendered name misses expressions consumed by conditionals or string formatting.
// Reading a whole map is conservative: any unresolved child makes that read unknown.
type staticNameInputs struct {
	section         map[string]any
	delims          delimiters.Delimiters
	legacyFunctions bool
	contextKnown    bool
}

func (s staticNameInputs) unresolvedNode(node parse.Node) bool {
	switch node := node.(type) {
	case *parse.ListNode:
		if node != nil {
			return s.unresolvedNodes(node.Nodes)
		}
	case *parse.ActionNode:
		return s.unresolvedNode(node.Pipe)
	case *parse.PipeNode:
		for _, command := range node.Cmds {
			if s.unresolvedNode(command) {
				return true
			}
		}
	case *parse.CommandNode:
		return s.unresolvedNodes(node.Args)
	default:
		return s.unresolvedReference(node)
	}
	return false
}

func (s staticNameInputs) unresolvedReference(node parse.Node) bool {
	switch node := node.(type) {
	case *parse.FieldNode:
		return !s.contextKnown && s.unresolvedField(node.Ident)
	case *parse.VariableNode:
		return len(node.Ident) > 0 && node.Ident[0] == "$" && s.unresolvedField(node.Ident[1:])
	case *parse.ChainNode:
		return s.unresolvedNode(node.Node)
	case *parse.DotNode:
		return !s.contextKnown && s.unresolvedValue(s.section)
	default:
		return s.unresolvedBranch(node)
	}
}

func (s staticNameInputs) unresolvedNodes(nodes []parse.Node) bool {
	for _, node := range nodes {
		if s.unresolvedNode(node) {
			return true
		}
	}
	return false
}

func (s staticNameInputs) unresolvedBranch(node parse.Node) bool {
	switch node := node.(type) {
	case *parse.IfNode:
		return s.unresolvedBranchReads(&node.BranchNode)
	case *parse.RangeNode:
		return s.unresolvedContextBranch(&node.BranchNode)
	case *parse.WithNode:
		return s.unresolvedContextBranch(&node.BranchNode)
	case *parse.TemplateNode:
		// A named template sees only its argument, including as $. Checking the whole
		// argument suffices: pure Go built-ins cannot introduce external values.
		return node.Pipe != nil && s.unresolvedNode(node.Pipe)
	}
	return false
}

func (s staticNameInputs) unresolvedBranchReads(node *parse.BranchNode) bool {
	return s.unresolvedNode(node.Pipe) || s.unresolvedNode(node.List) || s.unresolvedNode(node.ElseList)
}

// Range and with change dot only in their main body. Their else branch keeps
// the caller's context. Root-variable reads still use the original section.
func (s staticNameInputs) unresolvedContextBranch(node *parse.BranchNode) bool {
	if s.unresolvedNode(node.Pipe) || s.unresolvedNode(node.ElseList) {
		return true
	}
	body := s
	body.contextKnown = true
	return body.unresolvedNode(node.List)
}

func (s staticNameInputs) unresolvedField(path []string) bool {
	var value any = s.section
	for _, field := range path {
		values, ok := value.(map[string]any)
		if !ok {
			return s.unresolvedValue(value)
		}
		value = values[field]
	}
	return s.unresolvedValue(value)
}

func (s staticNameInputs) unresolvedValue(value any) bool {
	switch value := value.(type) {
	case string:
		return s.unresolvedString(value)
	case map[string]any:
		for _, child := range value {
			if s.unresolvedValue(child) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if s.unresolvedValue(child) {
				return true
			}
		}
	}
	return false
}

func (s staticNameInputs) unresolvedString(value string) bool {
	if _, literal := literalsource.Decode(value); literal {
		return false
	}
	return yamlsource.Contains(value) || s.delims.ContainsAny(value) ||
		(s.legacyFunctions && (fntag.MatchesStringForm(value) ||
			fntag.MatchesTag(value, fntag.ToYAML(fntag.Include)) || fntag.MatchesTag(value, fntag.ToYAML(fntag.IncludeRaw))))
}
