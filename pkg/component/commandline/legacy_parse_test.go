package commandline

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLegacyParseMatchesRequest(t *testing.T) {
	for _, tc := range []struct {
		name       string
		args       []string
		additional []string
		wantError  bool
	}{
		{name: "component", args: []string{"plan", "vpc", "--stack=dev"}},
		{name: "compound subcommand", args: []string{"state", "list", "vpc"}},
		{name: "forwarded arguments", args: []string{"plan", "vpc"}, additional: []string{"-lock=false"}},
		{name: "missing value", args: []string{"plan", "vpc", "--stack"}, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ProcessCommandLineArgs("terraform", legacyTestCommand(), tc.args, tc.additional)
			want, wantErr := Parse(Request{
				ComponentType: "terraform", Command: legacyTestCommand(),
				Args: tc.args, AdditionalArgsAndFlags: tc.additional,
			})
			if tc.wantError {
				require.Error(t, err)
				require.Error(t, wantErr)
				assert.Equal(t, wantErr.Error(), err.Error())
			} else {
				require.NoError(t, err)
				require.NoError(t, wantErr)
			}
			assert.Equal(t, want, got)
		})
	}
}

func legacyTestCommand() *cobra.Command {
	cmd := newTestCommandWithGlobalFlags("terraform")
	cmd.Flags().String("stack", "", "Stack name.")
	return cmd
}
