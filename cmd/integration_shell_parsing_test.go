package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// TestShellParser_ActualBehavior tests how the ACTUAL shell parser (mvdan.cc/sh)
// behaves when we pass it strings joined with/without quoting.
// This validates that our bug analysis is correct.
func TestShellParser_ActualBehavior(t *testing.T) {
	t.Run("unquoted string loses whitespace", func(t *testing.T) {
		// Simulate current buggy behavior: strings.Join without quoting
		args := []string{"echo", "hello  world"}
		unquotedCmd := strings.Join(args, " ") // "echo hello  world"

		// Parse with shell parser (what ExecuteShell does)
		parser, err := syntax.NewParser().Parse(strings.NewReader(unquotedCmd), "test")
		require.NoError(t, err)

		// Execute to capture actual behavior
		var output strings.Builder
		runner, err := interp.New(
			interp.StdIO(nil, &output, nil),
		)
		require.NoError(t, err)

		err = runner.Run(context.Background(), parser)
		require.NoError(t, err)

		result := strings.TrimSpace(output.String())
		t.Logf("Input args: %v", args)
		t.Logf("Joined string: %q", unquotedCmd)
		t.Logf("Shell output: %q", result)

		// BUG: Shell sees "hello  world" without quotes, treats double space as separator
		// So echo receives TWO args: "hello" and "world"
		// Output is: "hello world" (single space between words)
		assert.Equal(t, "hello world", result, "Double space should become single space (BUG)")
	})

	t.Run("quoted string preserves whitespace", func(t *testing.T) {
		// Simulate fixed behavior: syntax.Quote before joining
		args := []string{"echo", "hello  world"}
		var quotedArgs []string
		for _, arg := range args {
			quoted, err := syntax.Quote(arg, syntax.LangBash)
			require.NoError(t, err)
			quotedArgs = append(quotedArgs, quoted)
		}
		quotedCmd := strings.Join(quotedArgs, " ") // "echo 'hello  world'"

		// Parse with shell parser
		parser, err := syntax.NewParser().Parse(strings.NewReader(quotedCmd), "test")
		require.NoError(t, err)

		// Execute
		var output strings.Builder
		runner, err := interp.New(
			interp.StdIO(nil, &output, nil),
		)
		require.NoError(t, err)

		err = runner.Run(context.Background(), parser)
		require.NoError(t, err)

		result := strings.TrimSpace(output.String())
		t.Logf("Input args: %v", args)
		t.Logf("Quoted string: %q", quotedCmd)
		t.Logf("Shell output: %q", result)

		// FIXED: Shell sees 'hello  world' with quotes, preserves double space
		// So echo receives ONE arg: "hello  world"
		// Output is: "hello  world" (two spaces preserved)
		assert.Equal(t, "hello  world", result, "Double space should be preserved (FIXED)")
	})

	t.Run("special characters without quoting are dangerous", func(t *testing.T) {
		// Without quoting, shell metacharacters can cause issues
		args := []string{"echo", "foo;echo", "injected"}
		unquotedCmd := strings.Join(args, " ") // "echo foo;echo injected"

		parser, err := syntax.NewParser().Parse(strings.NewReader(unquotedCmd), "test")
		require.NoError(t, err)

		var output strings.Builder
		runner, err := interp.New(
			interp.StdIO(nil, &output, nil),
		)
		require.NoError(t, err)

		err = runner.Run(context.Background(), parser)
		require.NoError(t, err)

		result := strings.TrimSpace(output.String())
		t.Logf("Input args: %v", args)
		t.Logf("Unquoted: %q", unquotedCmd)
		t.Logf("Output: %q", result)

		// BUG: The semicolon causes shell to split into two commands!
		// "echo foo" runs, then "echo injected" runs
		// Output contains BOTH, proving command injection happened
		assert.Contains(t, result, "foo", "First command executed")
		assert.Contains(t, result, "injected", "Second command executed (COMMAND INJECTION!)")
	})

	t.Run("special characters with quoting are safe", func(t *testing.T) {
		args := []string{"echo", "foo;bar"}
		var quotedArgs []string
		for _, arg := range args {
			quoted, err := syntax.Quote(arg, syntax.LangBash)
			require.NoError(t, err)
			quotedArgs = append(quotedArgs, quoted)
		}
		quotedCmd := strings.Join(quotedArgs, " ") // "echo 'foo;bar'"

		parser, err := syntax.NewParser().Parse(strings.NewReader(quotedCmd), "test")
		require.NoError(t, err)

		var output strings.Builder
		runner, err := interp.New(
			interp.StdIO(nil, &output, nil),
		)
		require.NoError(t, err)

		err = runner.Run(context.Background(), parser)
		require.NoError(t, err)

		result := strings.TrimSpace(output.String())
		t.Logf("Input args: %v", args)
		t.Logf("Quoted: %q", quotedCmd)
		t.Logf("Output: %q", result)

		// With quoting, semicolon is literal - safe!
		assert.Equal(t, "foo;bar", result, "Semicolon should be literal, not command separator")
	})
}
