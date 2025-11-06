package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTerraformIdentityFlagCompletion tests that identity flag completion is registered for terraform commands.
func TestTerraformIdentityFlagCompletion(t *testing.T) {
	// Get terraform command.
	terraformCmd := RootCmd.Commands()
	var tfCmd *cobra.Command
	for _, cmd := range terraformCmd {
		if cmd.Name() == "terraform" || cmd.Name() == "tf" {
			tfCmd = cmd
			break
		}
	}

	require.NotNil(t, tfCmd, "terraform command should exist")

	// Check that identity flag exists (inherited from RootCmd persistent flags).
	// Use Flag() instead of PersistentFlags() because inherited flags won't show up in PersistentFlags().
	identityFlag := tfCmd.Flag("identity")
	require.NotNil(t, identityFlag, "identity flag should be defined on terraform command")

	// Check that completion function is registered.
	completionFunc, exists := tfCmd.GetFlagCompletionFunc("identity")
	assert.True(t, exists, "identity flag should have completion function registered")
	assert.NotNil(t, completionFunc, "completion function should not be nil")

	t.Log("✓ Identity flag completion is registered for terraform commands")
}
