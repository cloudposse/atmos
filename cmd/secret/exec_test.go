package secret

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSeparatedArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected []string
	}{
		{name: "no separator and no positional args", args: nil, expected: nil},
		{name: "positional args without separator are dropped", args: []string{"stray"}, expected: nil},
		{name: "single command after separator", args: []string{"--", "env"}, expected: []string{"env"}},
		{
			name:     "command with arguments after separator",
			args:     []string{"--", "aws", "s3", "ls"},
			expected: []string{"aws", "s3", "ls"},
		},
		{name: "separator with no following args yields nil", args: []string{"--"}, expected: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			require.NoError(t, cmd.ParseFlags(tt.args))

			result := getSeparatedArgs(cmd)

			if tt.expected == nil {
				assert.Nil(t, result)
				return
			}
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSecretExecCommand_Structure(t *testing.T) {
	assert.Equal(t, "exec -- <command> [args...]", execCmd.Use)
	assert.NotEmpty(t, execCmd.Short)
	assert.NotEmpty(t, execCmd.Long)
	assert.NotEmpty(t, execCmd.Example)
	assert.NotNil(t, execCmd.RunE)
	assert.False(t, execCmd.FParseErrWhitelist.UnknownFlags)
}
