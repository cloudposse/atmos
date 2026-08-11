package toolchain

import (
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
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

// TestLockCommand_RunE exercises the real lockCmd.RunE (not a re-implementation of its logic),
// covering the flag-binding -> max-concurrency resolution -> toolchain.RunLock wiring in
// cmd/toolchain/lock.go. It points VersionsFile at a non-existent path in an isolated temp dir
// so toolchain.RunLock's LoadToolVersions call fails deterministically instead of touching any
// real repo state, letting us assert the cmd layer propagates that error unwrapped.
func TestLockCommand_RunE(t *testing.T) {
	tempDir := t.TempDir()
	toolchain.SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: filepath.Join(tempDir, "tool-versions-does-not-exist"),
		},
	})
	t.Cleanup(func() { toolchain.SetAtmosConfig(nil) })

	tests := []struct {
		name           string
		maxConcurrency string
		wantErr        error
	}{
		{
			name:    "default flags reach RunLock and propagate its load failure",
			wantErr: errUtils.ErrToolVersionsFileOperation,
		},
		{
			name:           "invalid max-concurrency is rejected before RunLock is ever called",
			maxConcurrency: "0",
			wantErr:        errUtils.ErrInvalidFlagValue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() {
				require.NoError(t, lockCmd.Flags().Set("max-concurrency", "0"))
				// pflag.Set marks the flag Changed regardless of the value passed, and
				// resolveInstallMaxConcurrency keys off Changed (not just the value) to
				// decide whether the flag was explicitly passed. Reset it directly so the
				// next subtest starts from a truly-unset flag, not one that merely carries
				// the default value.
				lockCmd.Flags().Lookup("max-concurrency").Changed = false
				viper.Reset()
			})

			if tt.maxConcurrency != "" {
				require.NoError(t, lockCmd.Flags().Set("max-concurrency", tt.maxConcurrency))
			}

			err := lockCmd.RunE(lockCmd, []string{})

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
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
