package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestParseSeparatedArgs(t *testing.T) {
	tests := []struct {
		name              string
		cmdArgs           []string
		expectedAfterDash []string
		expectedHasSep    bool
		expectedDashPos   int
	}{
		{
			name:              "with separator and command",
			cmdArgs:           []string{"--", "echo", "hello"},
			expectedAfterDash: []string{"echo", "hello"},
			expectedHasSep:    true,
			expectedDashPos:   0,
		},
		{
			name:              "with flags before separator",
			cmdArgs:           []string{"--identity", "admin", "--", "terraform", "apply"},
			expectedAfterDash: []string{"terraform", "apply"},
			expectedHasSep:    true,
			expectedDashPos:   0, // Position in remaining args after --identity and admin are parsed
		},
		{
			name:              "no separator",
			cmdArgs:           []string{"--identity", "admin", "some", "args"},
			expectedAfterDash: nil,
			expectedHasSep:    false,
			expectedDashPos:   -1,
		},
		{
			name:              "separator with complex command",
			cmdArgs:           []string{"--identity", "dev", "--", "aws", "s3", "ls", "--recursive"},
			expectedAfterDash: []string{"aws", "s3", "ls", "--recursive"},
			expectedHasSep:    true,
			expectedDashPos:   0, // Position in remaining args after --identity dev are parsed
		},
		{
			name:              "separator with flags that look like values",
			cmdArgs:           []string{"--identity", "--", "--", "command", "--flag"},
			expectedAfterDash: []string{"command", "--flag"},
			expectedHasSep:    true,
			expectedDashPos:   0, // Position in remaining args after --identity -- are parsed
		},
		{
			name:              "only separator",
			cmdArgs:           []string{"--"},
			expectedAfterDash: []string{},
			expectedHasSep:    true,
			expectedDashPos:   0,
		},
		{
			name:              "empty args",
			cmdArgs:           []string{},
			expectedAfterDash: nil,
			expectedHasSep:    false,
			expectedDashPos:   -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test command with some flags.
			cmd := &cobra.Command{
				Use: "test",
				Run: func(cmd *cobra.Command, args []string) {},
			}
			cmd.Flags().String("identity", "", "test flag")
			cmd.Flags().String("other", "", "other flag")

			// Simulate Cobra's parsing by calling ParseFlags.
			// This will set ArgsLenAtDash if -- is present.
			err := cmd.ParseFlags(tt.cmdArgs)
			assert.NoError(t, err)

			// Get remaining args after Cobra parsed flags.
			remainingArgs := cmd.Flags().Args()

			// Parse separated args.
			separated := ParseSeparatedArgs(cmd, remainingArgs)

			// Verify results.
			assert.Equal(t, tt.expectedAfterDash, separated.AfterDash, "AfterDash mismatch")
			assert.Equal(t, tt.expectedHasSep, separated.HasSeparator(), "HasSeparator mismatch")
			assert.Equal(t, tt.expectedDashPos, separated.ArgsLenAtDash, "ArgsLenAtDash mismatch")
			assert.Equal(t, tt.expectedAfterDash, separated.CommandArgs(), "CommandArgs mismatch")
		})
	}
}

func TestParseSeparatedArgs_Integration(t *testing.T) {
	// This test simulates how a real command would use ParseSeparatedArgs.
	t.Run("simulates auth exec usage", func(t *testing.T) {
		executed := false
		var capturedIdentity string
		var capturedCommand []string

		cmd := &cobra.Command{
			Use: "exec [flags] -- COMMAND [args...]",
			RunE: func(cmd *cobra.Command, args []string) error {
				// Parse our own flags (Cobra did this automatically).
				identity, _ := cmd.Flags().GetString("identity")
				capturedIdentity = identity

				// Get arguments after --.
				separated := ParseSeparatedArgs(cmd, args)
				capturedCommand = separated.AfterDash

				executed = true
				return nil
			},
		}
		cmd.Flags().StringP("identity", "i", "", "Identity to use")

		// Simulate execution with: exec --identity admin -- terraform apply -auto-approve
		cmd.SetArgs([]string{"--identity", "admin", "--", "terraform", "apply", "-auto-approve"})
		err := cmd.Execute()

		assert.NoError(t, err)
		assert.True(t, executed)
		assert.Equal(t, "admin", capturedIdentity)
		assert.Equal(t, []string{"terraform", "apply", "-auto-approve"}, capturedCommand)
	})

	t.Run("simulates auth shell usage with optional args", func(t *testing.T) {
		executed := false
		var capturedIdentity string
		var capturedShell string
		var capturedShellArgs []string

		cmd := &cobra.Command{
			Use: "shell [flags] -- [SHELL_ARGS...]",
			RunE: func(cmd *cobra.Command, args []string) error {
				identity, _ := cmd.Flags().GetString("identity")
				shell, _ := cmd.Flags().GetString("shell")
				capturedIdentity = identity
				capturedShell = shell

				separated := ParseSeparatedArgs(cmd, args)
				capturedShellArgs = separated.AfterDash

				executed = true
				return nil
			},
		}
		cmd.Flags().StringP("identity", "i", "", "Identity to use")
		cmd.Flags().String("shell", "", "Shell to use")

		// Simulate: shell --identity dev --shell bash -- -c "echo test"
		cmd.SetArgs([]string{"--identity", "dev", "--shell", "bash", "--", "-c", "echo test"})
		err := cmd.Execute()

		assert.NoError(t, err)
		assert.True(t, executed)
		assert.Equal(t, "dev", capturedIdentity)
		assert.Equal(t, "bash", capturedShell)
		assert.Equal(t, []string{"-c", "echo test"}, capturedShellArgs)
	})

	t.Run("no separator provided", func(t *testing.T) {
		executed := false
		var capturedArgs []string

		cmd := &cobra.Command{
			Use: "exec [flags] -- COMMAND [args...]",
			RunE: func(cmd *cobra.Command, args []string) error {
				separated := ParseSeparatedArgs(cmd, args)
				capturedArgs = separated.AfterDash
				executed = true
				return nil
			},
		}
		cmd.Flags().String("identity", "", "Identity to use")

		// Simulate: exec --identity admin (no -- separator)
		cmd.SetArgs([]string{"--identity", "admin"})
		err := cmd.Execute()

		assert.NoError(t, err)
		assert.True(t, executed)
		assert.Nil(t, capturedArgs, "Should return nil when no separator")
	})
}

func TestSeparatedArgs_Methods(t *testing.T) {
	t.Run("HasSeparator returns true when separator exists", func(t *testing.T) {
		separated := &SeparatedArgs{
			AfterDash:     []string{"cmd"},
			ArgsLenAtDash: 1,
		}
		assert.True(t, separated.HasSeparator())
	})

	t.Run("HasSeparator returns false when no separator", func(t *testing.T) {
		separated := &SeparatedArgs{
			AfterDash:     nil,
			ArgsLenAtDash: -1,
		}
		assert.False(t, separated.HasSeparator())
	})

	t.Run("CommandArgs returns AfterDash", func(t *testing.T) {
		expected := []string{"terraform", "apply"}
		separated := &SeparatedArgs{
			AfterDash:     expected,
			ArgsLenAtDash: 2,
		}
		assert.Equal(t, expected, separated.CommandArgs())
	})
}
