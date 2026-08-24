package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/env"
)

// TestAuthEnvFormatCompletion verifies the format flag completion includes
// all supported formats (json + pkg/env.SupportedFormats). Ported from main's
// auth_integration_test.go (PR #1984).
func TestAuthEnvFormatCompletion(t *testing.T) {
	// Build expected formats: json + env.SupportedFormats.
	// JSON is handled separately in cmd/auth/env.go, not via pkg/env.
	expectedFormats := make([]string, 0, len(env.SupportedFormats)+1)
	expectedFormats = append(expectedFormats, "json")
	for _, f := range env.SupportedFormats {
		expectedFormats = append(expectedFormats, string(f))
	}

	t.Run("format flag provides correct completions", func(t *testing.T) {
		// Get the completion function for the format flag.
		completionFunc, exists := authEnvCmd.GetFlagCompletionFunc("format")
		require.True(t, exists, "Format flag should have completion function registered")

		// Call the completion function.
		completions, directive := completionFunc(authEnvCmd, []string{}, "")

		// Verify all supported formats are present.
		assert.ElementsMatch(t, expectedFormats, completions)
		assert.Equal(t, 4, int(directive)) // ShellCompDirectiveNoFileComp.
	})

	t.Run("format flag respects partial input", func(t *testing.T) {
		// Get the completion function.
		completionFunc, exists := authEnvCmd.GetFlagCompletionFunc("format")
		require.True(t, exists)

		// Call with partial input.
		completions, _ := completionFunc(authEnvCmd, []string{}, "js")

		// Should still return all formats (filtering is done by shell).
		assert.ElementsMatch(t, expectedFormats, completions)
	})
}

// TestAuthWhoamiOutputCompletion verifies that the --output flag of `atmos auth
// whoami` includes "json" in its completion. Ported from main's
// auth_integration_test.go (HEAD's StandardParser also surfaces "" as a valid
// value, so we assert membership instead of strict equality).
func TestAuthWhoamiOutputCompletion(t *testing.T) {
	t.Run("output flag provides json completion", func(t *testing.T) {
		// Get the completion function for the output flag.
		completionFunc, exists := authWhoamiCmd.GetFlagCompletionFunc("output")
		require.True(t, exists, "Output flag should have completion function registered")

		// Call the completion function.
		completions, directive := completionFunc(authWhoamiCmd, []string{}, "")

		// Verify json format is present.
		assert.Contains(t, completions, "json")
		assert.Equal(t, 4, int(directive)) // ShellCompDirectiveNoFileComp.
	})
}

// TestAuthCommandCompletion_FlagInheritance verifies the --identity persistent
// flag is wired and inherited by subcommands. Ported from main's
// auth_integration_test.go.
func TestAuthCommandCompletion_FlagInheritance(t *testing.T) {
	t.Run("auth command has persistent identity flag", func(t *testing.T) {
		// Verify auth command has identity flag.
		flag := authCmd.PersistentFlags().Lookup("identity")
		require.NotNil(t, flag)
		assert.Equal(t, "i", flag.Shorthand)

		// Verify completion function is registered.
		completionFunc, exists := authCmd.GetFlagCompletionFunc("identity")
		assert.True(t, exists)
		assert.NotNil(t, completionFunc)
	})

	t.Run("auth subcommands inherit identity completion", func(t *testing.T) {
		// Test that subcommands like auth login can access parent's persistent flag.
		flag := authLoginCmd.InheritedFlags().Lookup("identity")
		assert.NotNil(t, flag, "auth login should inherit identity flag")
	})
}
