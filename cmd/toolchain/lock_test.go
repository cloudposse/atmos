package toolchain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLockCommandProvider_UnusedInterfaceMethods covers the CommandProvider interface
// methods not already exercised by TestCommandProviderImplementations' table
// (GetCommand/GetName/GetGroup/GetFlagsBuilder).
func TestLockCommandProvider_UnusedInterfaceMethods(t *testing.T) {
	provider := &LockCommandProvider{}
	assert.Nil(t, provider.GetPositionalArgsBuilder())
	assert.Nil(t, provider.GetCompatibilityFlags())
}

// TestLockCommand_Flags tests lock command flags.
func TestLockCommand_Flags(t *testing.T) {
	t.Run("lock command has max-concurrency flag", func(t *testing.T) {
		flag := lockCmd.Flags().Lookup("max-concurrency")
		require.NotNil(t, flag)
		assert.Equal(t, "0", flag.DefValue)
	})
}

// TestLockCommand_CommandStructure tests the lock command structure.
func TestLockCommand_CommandStructure(t *testing.T) {
	assert.True(t, lockCmd.SilenceUsage)
	assert.True(t, lockCmd.SilenceErrors)
	assert.Contains(t, lockCmd.Use, "lock")
	assert.Contains(t, lockCmd.Use, "[tool...]")
}

// TestToolchainCommand_HasLockSubcommand verifies `lock` is registered on the parent
// `toolchain` command (i.e. discoverable via --help), not just defined as a standalone
// cobra.Command.
func TestToolchainCommand_HasLockSubcommand(t *testing.T) {
	found := false
	for _, c := range toolchainCmd.Commands() {
		if c.Name() == "lock" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected 'lock' to be registered as a toolchain subcommand")
}
