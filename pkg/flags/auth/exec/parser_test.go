package exec

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthExecParser_Reset(t *testing.T) {
	parser := NewAuthExecParser()

	cmd := &cobra.Command{
		Use: "exec",
	}
	parser.RegisterFlags(cmd)

	v := viper.New()
	err := parser.BindToViper(v)
	require.NoError(t, err)

	// Parse some args to set flag state
	ctx := context.Background()
	_, err = parser.Parse(ctx, []string{"--identity", "test", "command"})
	require.NoError(t, err)

	// Reset should clear the state
	parser.Reset()

	// After reset, flags should be in initial state
	// Verify by checking that a flag is not marked as changed
	identityFlag := cmd.Flags().Lookup("identity")
	require.NotNil(t, identityFlag)
	assert.False(t, identityFlag.Changed, "Flag should not be marked as changed after reset")
}

func TestAuthExecParser_Parse_WithIdentity(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "explicit identity",
			args:     []string{"--identity", "my-identity", "command"},
			expected: "my-identity",
		},
		{
			name:     "identity shorthand",
			args:     []string{"-i", "my-identity", "command"},
			expected: "my-identity",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewAuthExecParser()

			cmd := &cobra.Command{
				Use: "exec",
			}
			parser.RegisterFlags(cmd)

			v := viper.New()
			err := parser.BindToViper(v)
			require.NoError(t, err)

			ctx := context.Background()
			opts, err := parser.Parse(ctx, tt.args)

			require.NoError(t, err)
			require.NotNil(t, opts)
			assert.Equal(t, tt.expected, opts.Identity.Value())
		})
	}
}

func TestAuthExecParser_Parse_WithSeparatedArgs(t *testing.T) {
	parser := NewAuthExecParser()

	cmd := &cobra.Command{
		Use: "exec",
	}
	parser.RegisterFlags(cmd)

	v := viper.New()
	err := parser.BindToViper(v)
	require.NoError(t, err)

	ctx := context.Background()
	opts, err := parser.Parse(ctx, []string{"command", "--", "arg1", "arg2"})

	require.NoError(t, err)
	require.NotNil(t, opts)
	assert.Equal(t, []string{"command"}, opts.GetPositionalArgs())
	assert.Equal(t, []string{"arg1", "arg2"}, opts.GetSeparatedArgs())
}

func TestAuthExecParser_Parse_NoArgs(t *testing.T) {
	parser := NewAuthExecParser()

	cmd := &cobra.Command{
		Use: "exec",
	}
	parser.RegisterFlags(cmd)

	v := viper.New()
	err := parser.BindToViper(v)
	require.NoError(t, err)

	ctx := context.Background()
	opts, err := parser.Parse(ctx, []string{})

	require.NoError(t, err)
	require.NotNil(t, opts)
	assert.Empty(t, opts.GetPositionalArgs())
	assert.Empty(t, opts.GetSeparatedArgs())
	assert.True(t, opts.Identity.IsEmpty())
}

func TestAuthExecParser_RegisterFlags_IdentityFlag(t *testing.T) {
	parser := NewAuthExecParser()

	cmd := &cobra.Command{
		Use: "exec",
	}
	parser.RegisterFlags(cmd)

	// Verify identity flag is registered
	identityFlag := cmd.Flags().Lookup("identity")
	require.NotNil(t, identityFlag)
	assert.Equal(t, "identity", identityFlag.Name)
	assert.Equal(t, "i", identityFlag.Shorthand)
}

func TestAuthExecParser_MultipleReset(t *testing.T) {
	parser := NewAuthExecParser()

	cmd := &cobra.Command{
		Use: "exec",
	}
	parser.RegisterFlags(cmd)

	v := viper.New()
	err := parser.BindToViper(v)
	require.NoError(t, err)

	// Parse, reset, parse again
	ctx := context.Background()
	_, err = parser.Parse(ctx, []string{"--identity", "first", "command"})
	require.NoError(t, err)

	parser.Reset()

	_, err = parser.Parse(ctx, []string{"--identity", "second", "command"})
	require.NoError(t, err)

	// Both parses should succeed without errors
}
