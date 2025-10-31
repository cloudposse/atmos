package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"mvdan.cc/sh/v3/syntax"
)

// TestCustomCommands_WhitespaceBug demonstrates the CURRENT BUG in custom commands.
// When we use strings.Join() for trailing args, and then the shell parser re-parses,
// we LOSE argument boundaries and internal whitespace.
//
// This test documents the bug so we know what needs to be fixed.
func TestCustomCommands_WhitespaceBug(t *testing.T) {
	// Simulate what happens when user runs:
	// atmos mycmd run -- echo "hello  world" (two spaces)

	// After shell parsing, os.Args contains:
	osArgs := []string{"atmos", "mycmd", "run", "--", "echo", "hello  world"}

	// extractTrailingArgs joins with space:
	trailingArgsSlice := osArgs[4:] // ["echo", "hello  world"]
	trailingArgsString := strings.Join(trailingArgsSlice, " ") // "echo hello  world"

	assert.Equal(t, "echo hello  world", trailingArgsString, "Join preserves internal whitespace")

	// But then custom commands use this string in a template,
	// and ExecuteShell passes it to the shell parser:
	parser, err := syntax.NewParser().Parse(strings.NewReader(trailingArgsString), "test")
	assert.NoError(t, err)

	// What does the shell parser see?
	// It sees: echo hello  world (without quotes)
	// So it parses it as: ["echo", "hello", "world"] - whitespace is word separator!

	var words []string
	syntax.Walk(parser, func(node syntax.Node) bool {
		if word, ok := node.(*syntax.Word); ok {
			// Only get top-level words (command words)
			if lit, ok := word.Parts[0].(*syntax.Lit); ok {
				words = append(words, lit.Value)
			}
		}
		return true
	})

	// BUG: The shell parser sees 3 words, not 2!
	// Because we joined without shell quoting, "hello  world" became unquoted text.
	t.Logf("Shell parser extracted words: %v", words)
	t.Logf("Expected 2 words: [\"echo\", \"hello  world\"]")
	t.Logf("Got %d words: %v", len(words), words)

	// This demonstrates the bug - whitespace and argument boundaries are lost!
}

// TestCustomCommands_ProperShellQuoting shows how to FIX the bug using syntax.Quote.
func TestCustomCommands_ProperShellQuoting(t *testing.T) {
	// Same scenario:
	osArgs := []string{"atmos", "mycmd", "run", "--", "echo", "hello  world"}

	// CORRECT approach: Quote each argument and join:
	trailingArgsSlice := osArgs[4:] // ["echo", "hello  world"]

	var quotedArgs []string
	for _, arg := range trailingArgsSlice {
		// Quote each argument for shell safety
		quoted, err := syntax.Quote(arg, syntax.LangBash)
		assert.NoError(t, err)
		quotedArgs = append(quotedArgs, quoted)
	}

	trailingArgsString := strings.Join(quotedArgs, " ")

	t.Logf("Properly quoted string: %s", trailingArgsString)
	// This will be: echo 'hello  world' (with quotes!)

	// Now when the shell parser re-parses:
	parser, err := syntax.NewParser().Parse(strings.NewReader(trailingArgsString), "test")
	assert.NoError(t, err)

	// It correctly sees 2 words with preserved whitespace
	var words []string
	syntax.Walk(parser, func(node syntax.Node) bool {
		// This is simplified - real parsing would need to handle quoted strings properly
		if word, ok := node.(*syntax.Word); ok {
			// Get the literal value
			if len(word.Parts) > 0 {
				if lit, ok := word.Parts[0].(*syntax.Lit); ok {
					words = append(words, lit.Value)
				} else if sq, ok := word.Parts[0].(*syntax.SglQuoted); ok {
					// Handle single-quoted string
					words = append(words, sq.Value)
				}
			}
		}
		return true
	})

	t.Logf("With quoting, parser extracted: %v", words)
	assert.Equal(t, 2, len(words), "Should parse as 2 words")
}
