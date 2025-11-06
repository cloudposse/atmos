package exec

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuthExecParserIntegration tests the migrated parser with AtmosFlagParser.
func TestAuthExecParserIntegration(t *testing.T) {
	tests := []struct {
		name             string
		args             []string
		expectedIdentity string
		expectedPosArgs  []string
		expectedSepArgs  []string
	}{
		{
			name:             "with identity flag (equals syntax required with NoOptDefVal)",
			args:             []string{"-i=prod", "--", "aws", "s3", "ls"},
			expectedIdentity: "prod",
			expectedPosArgs:  []string{},
			expectedSepArgs:  []string{"aws", "s3", "ls"},
		},
		{
			name:             "without identity flag",
			args:             []string{"--", "aws", "s3", "ls"},
			expectedIdentity: "",
			expectedPosArgs:  []string{},
			expectedSepArgs:  []string{"aws", "s3", "ls"},
		},
		{
			name:             "identity with equals syntax",
			args:             []string{"--identity=staging", "--", "terraform", "plan"},
			expectedIdentity: "staging",
			expectedPosArgs:  []string{},
			expectedSepArgs:  []string{"terraform", "plan"},
		},
		{
			name:             "command without separator",
			args:             []string{"aws", "s3", "ls"},
			expectedIdentity: "",
			expectedPosArgs:  []string{"aws", "s3", "ls"},
			expectedSepArgs:  []string{},
		},
		{
			name:             "with identity flag using space syntax (preprocessor converts to equals)",
			args:             []string{"-i", "prod", "--", "aws", "s3", "ls"},
			expectedIdentity: "prod",
			expectedPosArgs:  []string{},
			expectedSepArgs:  []string{"aws", "s3", "ls"},
		},
		{
			name:             "with identity flag using space syntax without separator",
			args:             []string{"--identity", "staging", "terraform", "plan"},
			expectedIdentity: "staging",
			expectedPosArgs:  []string{"terraform", "plan"},
			expectedSepArgs:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			cmd := &cobra.Command{Use: "exec"}
			v := viper.New()
			parser := NewAuthExecParser()

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
			assert.Equal(t, tt.expectedPosArgs, opts.PositionalArgs, "positional args mismatch")
			assert.Equal(t, tt.expectedSepArgs, opts.SeparatedArgs, "separated args mismatch")
		})
	}
}
