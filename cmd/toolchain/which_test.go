package toolchain

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Provider and structure tests are in command_provider_test.go.
// This file contains which-specific tests.

func TestWhichCommand_Example(t *testing.T) {
	t.Run("Example is set", func(t *testing.T) {
		assert.NotEmpty(t, whichCmd.Example)
		assert.Contains(t, whichCmd.Example, "terraform")
	})
}

func TestWhichCommand_Args(t *testing.T) {
	t.Run("requires exactly one argument", func(t *testing.T) {
		require.NotNil(t, whichCmd.Args)

		// Test that no args fails.
		err := whichCmd.Args(whichCmd, []string{})
		assert.Error(t, err)

		// Test that one arg succeeds.
		err = whichCmd.Args(whichCmd, []string{"terraform"})
		assert.NoError(t, err)

		// Test that two args fails.
		err = whichCmd.Args(whichCmd, []string{"terraform", "extra"})
		assert.Error(t, err)
	})
}

func TestWhichCommand_ValidatesArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name:    "valid single tool",
			args:    []string{"terraform"},
			wantErr: false,
		},
		{
			name:    "valid tool with org",
			args:    []string{"hashicorp/terraform"},
			wantErr: false,
		},
		{
			name:    "no arguments",
			args:    []string{},
			wantErr: true,
		},
		{
			name:    "too many arguments",
			args:    []string{"terraform", "helm"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testCmd := &cobra.Command{
				Use:  "which <tool>",
				Args: cobra.ExactArgs(1),
			}

			err := testCmd.Args(testCmd, tt.args)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
