package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunCompletion(t *testing.T) {
	tests := []struct {
		name    string
		use     string
		wantErr bool
	}{
		{
			name:    "bash completion",
			use:     "bash",
			wantErr: false,
		},
		{
			name:    "zsh completion",
			use:     "zsh",
			wantErr: false,
		},
		{
			name:    "fish completion",
			use:     "fish",
			wantErr: false,
		},
		{
			name:    "powershell completion",
			use:     "powershell",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a root command for testing
			rootCmd := &cobra.Command{
				Use: "atmos",
			}

			// Create a test command with the specific use case
			cmd := &cobra.Command{
				Use: tt.use,
			}
			rootCmd.AddCommand(cmd)

			// Run the completion function
			err := runCompletion(cmd, []string{})

			if tt.wantErr {
				require.Error(t, err)
			} else {
				// Just check that it doesn't error - output checking is complex
				require.NoError(t, err)
			}
		})
	}
}

func TestCompletionCmd(t *testing.T) {
	// Test that completion command exists and has subcommands
	assert.NotNil(t, completionCmd)
	assert.Equal(t, "completion [bash|zsh|fish|powershell]", completionCmd.Use)
	assert.True(t, completionCmd.HasSubCommands())

	// Test that all shell subcommands exist
	shells := []string{"bash", "zsh", "fish", "powershell"}
	for _, shell := range shells {
		t.Run("has_"+shell+"_subcommand", func(t *testing.T) {
			cmd, _, err := completionCmd.Find([]string{shell})
			require.NoError(t, err)
			assert.Equal(t, shell, cmd.Use)
		})
	}
}
