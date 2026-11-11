package main

import (
	"strings"
	"text/template/parse"
)

// guardEnvironmentOutput checks rendered actions before text/template uses fmt.
// Control and assignment pipelines retain maps for indexing and iteration.
func guardEnvironmentOutput(node parse.Node) {
	if node == nil {
		return
	}
	switch node := node.(type) {
	case *parse.ListNode:
		if node != nil {
			for _, child := range node.Nodes {
				guardEnvironmentOutput(child)
			}
		}
	case *parse.ActionNode:
		if len(node.Pipe.Decl) == 0 {
			node.Pipe.Cmds = append(node.Pipe.Cmds, &parse.CommandNode{
				NodeType: parse.NodeCommand,
				Args:     []parse.Node{parse.NewIdentifier("_environmentOutput")},
			})
		}
	case *parse.IfNode:
		guardEnvironmentBranch(node.BranchNode)
	case *parse.RangeNode:
		guardEnvironmentBranch(node.BranchNode)
	case *parse.WithNode:
		guardEnvironmentBranch(node.BranchNode)
	}
}

func guardEnvironmentBranch(node parse.BranchNode) {
	guardEnvironmentOutput(node.List)
	guardEnvironmentOutput(node.ElseList)
}

func environmentOutput(value any) (any, error) {
	switch value := value.(type) {
	case string:
		if len(value) > maxEnvironmentValueBytes || strings.ContainsRune(value, 0) {
			return nil, errEnvironmentTemplate
		}
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, uintptr, float32, float64, complex64, complex128:
	default:
		return nil, errEnvironmentTemplate
	}
	return value, nil
}
