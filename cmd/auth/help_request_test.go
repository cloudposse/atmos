package auth

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseLikeAuthSubcommand runs argv through Cobra's real flag parsing and
// returns the positional args and the command, exactly as RunE receives them.
// Cobra strips the `--` separator from args and records its position in
// ArgsLenAtDash, which is what helpRequested must honor.
func parseLikeAuthSubcommand(t *testing.T, argv []string) (*cobra.Command, []string) {
	t.Helper()

	var gotCmd *cobra.Command
	var gotArgs []string
	cmd := &cobra.Command{
		Use: "exec",
		RunE: func(c *cobra.Command, args []string) error {
			gotCmd, gotArgs = c, args
			return nil
		},
	}
	cmd.Flags().String("identity", "", "identity")
	cmd.SetArgs(argv)
	require.NoError(t, cmd.Execute())
	require.NotNil(t, gotCmd)
	return gotCmd, gotArgs
}

func TestHelpRequestedIgnoresArgsAfterSeparator(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want bool
	}{
		{name: "short help flag of the child command", argv: []string{"--", "df", "-h"}, want: false},
		{name: "long help flag of the child command", argv: []string{"--identity", "prod", "--", "aws", "s3", "ls", "--help"}, want: false},
		{name: "child command without help", argv: []string{"--", "terraform", "plan"}, want: false},
		{name: "positional before the separator", argv: []string{"--identity=prod", "positional", "--", "df"}, want: false},
		{name: "no separator", argv: []string{"positional"}, want: false},
		{name: "empty", argv: []string{}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, args := parseLikeAuthSubcommand(t, tt.argv)
			assert.Equal(t, tt.want, helpRequested(cmd, args), "args=%v dash=%d", args, cmd.ArgsLenAtDash())
		})
	}
}

// TestHelpRequestedDetectsHelpBeforeSeparator covers args that still carry a
// help token for Atmos itself (for example when flag parsing left it in args).
func TestHelpRequestedDetectsHelpBeforeSeparator(t *testing.T) {
	cmd := &cobra.Command{Use: "exec"}
	assert.True(t, helpRequested(cmd, []string{"-h"}))
	assert.True(t, helpRequested(cmd, []string{"positional", "--help"}))
	// A literal separator in args (flag parsing disabled) still stops the scan.
	assert.False(t, helpRequested(cmd, []string{"--", "df", "-h"}))
}
