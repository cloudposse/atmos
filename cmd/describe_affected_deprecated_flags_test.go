package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDescribeAffectedDeprecatedFlagsWarnOnStderr keeps --ref/--sha deprecation notices off the
// command output writer (stdout), which carries pipeable --format json/yaml output.
func TestDescribeAffectedDeprecatedFlagsWarnOnStderr(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{"ref", []string{"--ref", "refs/heads/main"}, []string{"--ref"}},
		{"sha", []string{"--sha", "abc123"}, []string{"--sha"}},
		{"both", []string{"--ref", "main", "--sha", "abc123"}, []string{"--ref", "--sha"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_ = NewTestKit(t)
			var commandOutput bytes.Buffer
			describeAffectedCmd.SetOut(&commandOutput)
			t.Cleanup(func() { describeAffectedCmd.SetOut(nil) })

			stdout, stderr := captureStdoutStderr(t, func() {
				ensureIOInitialized(t)
				require.NoError(t, describeAffectedCmd.ParseFlags(tc.args))
				warnDeprecatedDescribeAffectedFlags(describeAffectedCmd)
			})
			assert.Empty(t, commandOutput.String(), "deprecation notices must not reach the command output writer")
			assert.Empty(t, stdout)
			for _, flag := range tc.want {
				assert.Contains(t, stderr, "Flag "+flag+" has been deprecated, use --base instead")
			}
		})
	}
}

func TestDescribeAffectedWithoutDeprecatedFlagsDoesNotWarn(t *testing.T) {
	_ = NewTestKit(t)
	stdout, stderr := captureStdoutStderr(t, func() {
		ensureIOInitialized(t)
		require.NoError(t, describeAffectedCmd.ParseFlags([]string{"--base", "main"}))
		warnDeprecatedDescribeAffectedFlags(describeAffectedCmd)
	})
	assert.Empty(t, stdout)
	assert.NotContains(t, stderr, "deprecated")
}

func TestDescribeAffectedDeprecatedFlagsStayHidden(t *testing.T) {
	for _, name := range []string{"ref", "sha"} {
		flag := describeAffectedCmd.PersistentFlags().Lookup(name)
		require.NotNil(t, flag, name)
		assert.True(t, flag.Hidden, "--%s must stay out of help like a deprecated flag", name)
	}
}
