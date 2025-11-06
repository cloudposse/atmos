package packer

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParserIntegration tests the migrated parser with AtmosFlagParser.
func TestParserIntegration(t *testing.T) {
	tests := []struct {
		name              string
		args              []string
		expectedStack     string
		expectedComponent string
		expectedSepArgs   []string
		expectedPosArgs   []string
	}{
		{
			name:              "basic usage with stack and component",
			args:              []string{"ami", "-s", "dev"},
			expectedStack:     "dev",
			expectedComponent: "ami",
			expectedPosArgs:   []string{"ami"},
			expectedSepArgs:   []string{},
		},
		{
			name:              "with separated args after --",
			args:              []string{"docker", "--stack=prod", "--", "-var", "foo=bar"},
			expectedStack:     "prod",
			expectedComponent: "docker",
			expectedPosArgs:   []string{"docker"},
			expectedSepArgs:   []string{"-var", "foo=bar"},
		},
		{
			name:              "component only",
			args:              []string{"image"},
			expectedStack:     "",
			expectedComponent: "image",
			expectedPosArgs:   []string{"image"},
			expectedSepArgs:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			cmd := &cobra.Command{Use: "packer"}
			v := viper.New()
			parser := NewParser()

			// Register flags
			parser.RegisterFlags(cmd)
			err := parser.BindToViper(v)
			require.NoError(t, err)

			// Parse
			opts, err := parser.Parse(context.Background(), tt.args)
			require.NoError(t, err)
			require.NotNil(t, opts)

			// Verify
			assert.Equal(t, tt.expectedStack, opts.Stack, "stack mismatch")
			assert.Equal(t, tt.expectedComponent, opts.Component, "component mismatch")
			assert.Equal(t, tt.expectedPosArgs, opts.GetPositionalArgs(), "positional args mismatch")
			assert.Equal(t, tt.expectedSepArgs, opts.GetSeparatedArgs(), "separated args mismatch")
		})
	}
}
