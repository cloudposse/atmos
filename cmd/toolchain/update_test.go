package toolchain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateCommandProvider_UnusedInterfaceMethods covers the CommandProvider
// interface methods not already exercised by TestCommandProviderImplementations'
// table (GetCommand/GetName/GetGroup/GetFlagsBuilder), so behavior isn't
// silently dropped without re-deriving that table's near-identical per-provider
// test body here.
func TestUpdateCommandProvider_UnusedInterfaceMethods(t *testing.T) {
	provider := &UpdateCommandProvider{}
	assert.Nil(t, provider.GetPositionalArgsBuilder())
	assert.Nil(t, provider.GetCompatibilityFlags())
}

// TestUpdateCommand_Flags tests update command flags.
func TestUpdateCommand_Flags(t *testing.T) {
	t.Run("update command has dry-run flag", func(t *testing.T) {
		flag := updateCmd.Flags().Lookup("dry-run")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("update command has max-concurrency flag", func(t *testing.T) {
		flag := updateCmd.Flags().Lookup("max-concurrency")
		require.NotNil(t, flag)
		assert.Equal(t, "0", flag.DefValue)
	})
}

// TestUpdateCommand_CommandStructure tests the update command structure.
func TestUpdateCommand_CommandStructure(t *testing.T) {
	assert.True(t, updateCmd.SilenceUsage)
	assert.True(t, updateCmd.SilenceErrors)
	assert.Contains(t, updateCmd.Use, "update")
	assert.Contains(t, updateCmd.Use, "[tool...]")
}

// TestUpdateCommand_HelpTextDoesNotClaimRefPinsAreImmutable reproduces a field-test finding:
// the PR's own follow-up commit (and the published toolchain-update.mdx docs) explicitly
// corrected the claim that ref:-pinned tools are skipped because they're "immutable by
// design" -- a named git ref CAN move; it's skipped to preserve the user's explicit source
// selection, not because it can't change. The CLI's own --help text (this command's Long
// description) was never updated to match and still asserts blanket immutability, which end
// users see far more often than the website docs.
func TestUpdateCommand_HelpTextDoesNotClaimRefPinsAreImmutable(t *testing.T) {
	assert.NotContains(t, updateCmd.Long, "immutable by design",
		"a named ref: pin can move -- it's skipped by choice, not because it's immutable; the --help text should say so, not the opposite")
}

// TestToolchainCommand_HasUpdateSubcommand verifies `update` is registered on
// the parent `toolchain` command (i.e. discoverable via --help), not just
// defined as a standalone cobra.Command.
func TestToolchainCommand_HasUpdateSubcommand(t *testing.T) {
	found := false
	for _, c := range toolchainCmd.Commands() {
		if c.Name() == "update" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected 'update' to be registered as a toolchain subcommand")
}
