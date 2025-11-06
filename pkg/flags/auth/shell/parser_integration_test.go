package shell

import (
	"context"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuthShellParserIntegration tests the migrated parser with AtmosFlagParser.
func TestAuthShellParserIntegration(t *testing.T) {
	tests := []struct {
		name             string
		args             []string
		expectedIdentity string
		expectedShell    string
		expectedPosArgs  []string
		expectedSepArgs  []string
	}{
		{
			name:             "with identity and shell flags",
			args:             []string{"-i", "prod", "-s", "bash"},
			expectedIdentity: "prod",
			expectedShell:    "bash",
			expectedPosArgs:  []string{},
			expectedSepArgs:  []string{},
		},
		{
			name:             "identity only",
			args:             []string{"--identity=staging"},
			expectedIdentity: "staging",
			expectedShell:    "",
			expectedPosArgs:  []string{},
			expectedSepArgs:  []string{},
		},
		{
			name:             "shell only",
			args:             []string{"--shell=zsh"},
			expectedIdentity: "",
			expectedShell:    "zsh",
			expectedPosArgs:  []string{},
			expectedSepArgs:  []string{},
		},
		{
			name:             "no flags",
			args:             []string{},
			expectedIdentity: "",
			expectedShell:    "",
			expectedPosArgs:  []string{},
			expectedSepArgs:  []string{},
		},
		{
			name:             "with args after separator",
			args:             []string{"-i", "prod", "--", "-c", "echo hello"},
			expectedIdentity: "prod",
			expectedShell:    "",
			expectedPosArgs:  []string{},
			expectedSepArgs:  []string{"-c", "echo hello"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear SHELL env var to avoid picking up host shell
			oldShell := os.Getenv("SHELL")
			os.Setenv("SHELL", "")
			defer os.Setenv("SHELL", oldShell)

			// Setup
			cmd := &cobra.Command{Use: "shell"}
			v := viper.New()
			parser := NewAuthShellParser()

			// Register flags
			parser.RegisterFlags(cmd)
			err := parser.BindToViper(v)
			require.NoError(t, err)

			// Parse
			opts, err := parser.Parse(context.Background(), tt.args)
			require.NoError(t, err)
			require.NotNil(t, opts)

			// Verify
			assert.Equal(t, tt.expectedIdentity, opts.Identity.Value(), "identity mismatch")
			assert.Equal(t, tt.expectedShell, opts.Shell, "shell mismatch")
			assert.Equal(t, tt.expectedPosArgs, opts.PositionalArgs, "positional args mismatch")
			assert.Equal(t, tt.expectedSepArgs, opts.SeparatedArgs, "separated args mismatch")
		})
	}
}
