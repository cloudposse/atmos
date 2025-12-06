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

	// Check that identity flag exists.
	// The identity flag is registered as a persistent flag on the terraform command.
	identityFlag := tfCmd.PersistentFlags().Lookup("identity")
	if identityFlag == nil {
		// Fallback to local flags.
		identityFlag = tfCmd.LocalFlags().Lookup("identity")
	}
	if identityFlag == nil {
		// Fallback to inherited flags from parent.
		identityFlag = tfCmd.InheritedFlags().Lookup("identity")
	}
	require.NotNil(t, identityFlag, "identity flag should be available on terraform command (directly or inherited)")

	// Check that completion function is registered on terraform command or parent.
	completionFunc, exists := tfCmd.GetFlagCompletionFunc("identity")
	if !exists {
		// Completion may be registered on parent command (RootCmd).
		completionFunc, exists = RootCmd.GetFlagCompletionFunc("identity")
	}
	assert.True(t, exists, "identity flag should have completion function registered (on terraform or root)")
	assert.NotNil(t, completionFunc, "completion function should not be nil")

	t.Log("✓ Identity flag completion is registered for terraform commands")
}
