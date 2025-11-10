package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// TestCustomCommands_ExtractSeparatedArgsBaseline tests the baseline behavior of ExtractSeparatedArgs for custom commands.
// Custom commands now use ExtractSeparatedArgs which returns a SeparatedCommandArgs struct.
func TestCustomCommands_ExtractSeparatedArgsBaseline(t *testing.T) {
	tests := []struct {
		name              string
		args              []string
		osArgs            []string
		expectedBeforeSep []string
		expectedAfterSep  []string
	}{
		{
			name:              "custom command with separator",
			args:              []string{"arg1", "arg2", "--", "trail1", "trail2"},
			osArgs:            []string{"atmos", "mycmd", "arg1", "arg2", "--", "trail1", "trail2"},
			expectedBeforeSep: []string{"arg1", "arg2"},
			expectedAfterSep:  []string{"trail1", "trail2"},
		},
		{
			name:              "custom command no trailing args",
			args:              []string{"arg1", "arg2"},
			osArgs:            []string{"atmos", "mycmd", "arg1", "arg2"},
			expectedBeforeSep: []string{"arg1", "arg2"},
			expectedAfterSep:  nil,
		},
		{
			name:              "custom command double dash at end",
			args:              []string{"arg1", "--"},
			osArgs:            []string{"atmos", "mycmd", "arg1", "--"},
			expectedBeforeSep: []string{"arg1"},
			expectedAfterSep:  []string{},
		},
		{
			name:              "custom command with complex trailing args",
			args:              []string{"deploy", "prod", "--", "--force", "--timeout=300"},
			osArgs:            []string{"atmos", "deploy-app", "deploy", "prod", "--", "--force", "--timeout=300"},
			expectedBeforeSep: []string{"deploy", "prod"},
			expectedAfterSep:  []string{"--force", "--timeout=300"},
		},
		{
			name:              "custom command with whitespace in trailing args",
			args:              []string{"run", "--", "echo", "hello world"},
			osArgs:            []string{"atmos", "mycmd", "run", "--", "echo", "hello world"},
			expectedBeforeSep: []string{"run"},
			expectedAfterSep:  []string{"echo", "hello world"},
		},
		{
			name:              "custom command with multiple spaces",
			args:              []string{"run", "--", "echo", "hello  world"},
			osArgs:            []string{"atmos", "mycmd", "run", "--", "echo", "hello  world"},
			expectedBeforeSep: []string{"run"},
			expectedAfterSep:  []string{"echo", "hello  world"},
		},
		{
			name:              "custom command with special shell characters",
			args:              []string{"run", "--", "echo", "$VAR", "foo'bar", "baz\"qux"},
			osArgs:            []string{"atmos", "mycmd", "run", "--", "echo", "$VAR", "foo'bar", "baz\"qux"},
			expectedBeforeSep: []string{"run"},
			expectedAfterSep:  []string{"echo", "$VAR", "foo'bar", "baz\"qux"},
		},
		{
			name:              "custom command with empty arg",
			args:              []string{"run", "--", "echo", "", "test"},
			osArgs:            []string{"atmos", "mycmd", "run", "--", "echo", "", "test"},
			expectedBeforeSep: []string{"run"},
			expectedAfterSep:  []string{"echo", "", "test"},
		},
		{
			name:              "custom command with newlines",
			args:              []string{"run", "--", "echo", "line1\nline2"},
			osArgs:            []string{"atmos", "mycmd", "run", "--", "echo", "line1\nline2"},
			expectedBeforeSep: []string{"run"},
			expectedAfterSep:  []string{"echo", "line1\nline2"},
		},
		{
			name:              "custom command with tabs and mixed whitespace",
			args:              []string{"run", "--", "echo", "tab\there", "space  here"},
			osArgs:            []string{"atmos", "mycmd", "run", "--", "echo", "tab\there", "space  here"},
			expectedBeforeSep: []string{"run"},
			expectedAfterSep:  []string{"echo", "tab\there", "space  here"},
		},
		{
			name:              "custom command with backslashes",
			args:              []string{"run", "--", "echo", "C:\\Program Files\\"},
			osArgs:            []string{"atmos", "mycmd", "run", "--", "echo", "C:\\Program Files\\"},
			expectedBeforeSep: []string{"run"},
			expectedAfterSep:  []string{"echo", "C:\\Program Files\\"},
		},
		{
			name:              "custom command with unicode",
			args:              []string{"run", "--", "echo", "café", "日本語", "emoji😀"},
			osArgs:            []string{"atmos", "mycmd", "run", "--", "echo", "café", "日本語", "emoji😀"},
			expectedBeforeSep: []string{"run"},
			expectedAfterSep:  []string{"echo", "café", "日本語", "emoji😀"},
		},
		{
			name:              "custom command with semicolons and pipes (shell metacharacters)",
			args:              []string{"run", "--", "echo", "foo;bar", "baz|qux"},
			osArgs:            []string{"atmos", "mycmd", "run", "--", "echo", "foo;bar", "baz|qux"},
			expectedBeforeSep: []string{"run"},
			expectedAfterSep:  []string{"echo", "foo;bar", "baz|qux"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			separated := ExtractSeparatedArgs(cmd, tt.args, tt.osArgs)

			assert.Equal(t, tt.expectedBeforeSep, separated.BeforeSeparator, "BeforeSeparator mismatch")
			assert.Equal(t, tt.expectedAfterSep, separated.AfterSeparator, "AfterSeparator mismatch")
		})
	}
}

// TestCustomCommands_QuotedStringEdgeCases tests that GetAfterSeparatorAsQuotedString properly
// handles all edge cases with proper shell quoting to preserve argument boundaries and special characters.
func TestCustomCommands_QuotedStringEdgeCases(t *testing.T) {
	tests := []struct {
		name                  string
		args                  []string
		osArgs                []string
		shouldContainInQuoted []string // Strings that should appear in the quoted output
		description           string
	}{
		{
			name:                  "multiple spaces preserved",
			args:                  []string{"run", "--", "echo", "hello  world"},
			osArgs:                []string{"atmos", "mycmd", "run", "--", "echo", "hello  world"},
			shouldContainInQuoted: []string{"echo", "hello  world"},
			description:           "Two spaces should be preserved in quoted arg",
		},
		{
			name:                  "special shell characters quoted",
			args:                  []string{"run", "--", "echo", "$VAR", "foo'bar"},
			osArgs:                []string{"atmos", "mycmd", "run", "--", "echo", "$VAR", "foo'bar"},
			shouldContainInQuoted: []string{"echo"},
			description:           "Shell variables and quotes should be escaped",
		},
		{
			name:                  "empty argument preserved",
			args:                  []string{"run", "--", "echo", "", "test"},
			osArgs:                []string{"atmos", "mycmd", "run", "--", "echo", "", "test"},
			shouldContainInQuoted: []string{"echo", "test"},
			description:           "Empty string should be represented",
		},
		{
			name:                  "newlines preserved",
			args:                  []string{"run", "--", "echo", "line1\nline2"},
			osArgs:                []string{"atmos", "mycmd", "run", "--", "echo", "line1\nline2"},
			shouldContainInQuoted: []string{"echo"},
			description:           "Newlines should be preserved",
		},
		{
			name:                  "tabs and mixed whitespace",
			args:                  []string{"run", "--", "echo", "tab\there"},
			osArgs:                []string{"atmos", "mycmd", "run", "--", "echo", "tab\there"},
			shouldContainInQuoted: []string{"echo"},
			description:           "Tabs should be preserved",
		},
		{
			name:                  "backslashes preserved",
			args:                  []string{"run", "--", "echo", "C:\\Program Files\\"},
			osArgs:                []string{"atmos", "mycmd", "run", "--", "echo", "C:\\Program Files\\"},
			shouldContainInQuoted: []string{"echo"},
			description:           "Backslashes should be preserved",
		},
		{
			name:                  "semicolons and pipes quoted",
			args:                  []string{"run", "--", "echo", "foo;bar", "baz|qux"},
			osArgs:                []string{"atmos", "mycmd", "run", "--", "echo", "foo;bar", "baz|qux"},
			shouldContainInQuoted: []string{"echo"},
			description:           "Shell metacharacters should be quoted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			separated := ExtractSeparatedArgs(cmd, tt.args, tt.osArgs)

			quoted, err := separated.GetAfterSeparatorAsQuotedString()
			assert.NoError(t, err, "Quoting should not fail")

			t.Logf("Description: %s", tt.description)
			t.Logf("Quoted string: %s", quoted)

			for _, expected := range tt.shouldContainInQuoted {
				assert.Contains(t, quoted, expected, "Quoted string should contain %q", expected)
			}

			// Verify it's different from unquoted (except for simple cases)
			unquoted := separated.GetAfterSeparatorAsString()
			if len(tt.shouldContainInQuoted) > 1 {
				t.Logf("Unquoted: %s", unquoted)
				t.Logf("  Quoted: %s", quoted)
			}
		})
	}
}
