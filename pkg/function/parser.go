package function

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

const wsChars = " \t\n\r"

// isExpressionStart returns true if the character starts an expression.
// Expressions start with: . (dot access), | (pipe), [ (bracket), { (JSON/dict),
// " or ' (string literals).
func isExpressionStart(c byte) bool {
	switch c {
	case '.', '|', '[', '{', '"', '\'':
		return true
	default:
		return false
	}
}

// ParseArgs parses YAML function arguments.
// Format: component [stack] expression
//
// Rules:
//   - Component is the first word (no spaces allowed in component names)
//   - If the next token starts with . | [ { " ' it's treated as the expression
//   - Otherwise, the second word is the stack, and everything after is the expression
//   - Handles any amount of whitespace (spaces, tabs, newlines) between tokens
//
// Examples:
//
//	"component-1 vpc_id"                    -> component="component-1", stack="", expr="vpc_id"
//	"component-1 prod vpc_id"               -> component="component-1", stack="prod", expr="vpc_id"
//	"component-2 .output"                   -> component="component-2", stack="", expr=".output"
//	"component-2 prod .output"              -> component="component-2", stack="prod", expr=".output"
//	`component-2 .output // {"key": "val"}` -> component="component-2", stack="", expr=`.output // {"key": "val"}`
func ParseArgs(input string) (component, stack, expr string) {
	defer perf.Track(nil, "function.ParseArgs")()

	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", ""
	}

	// Find the first whitespace to extract component.
	idx := strings.IndexAny(input, wsChars)
	if idx == -1 {
		return input, "", "" // Just component.
	}
	component = input[:idx]

	// Skip all whitespace after component.
	rest := strings.TrimLeft(input[idx:], wsChars)
	if rest == "" {
		return component, "", ""
	}

	// Check if rest starts with an expression character.
	if isExpressionStart(rest[0]) {
		return component, "", rest
	}

	// Look for next whitespace to find potential stack.
	idx = strings.IndexAny(rest, wsChars)
	if idx == -1 {
		// Only one more word - it's the expression (simple output name).
		return component, "", rest
	}

	// Two+ words remaining: first is stack, rest is expression.
	stack = rest[:idx]
	expr = strings.TrimLeft(rest[idx:], wsChars)

	return component, stack, expr
}
