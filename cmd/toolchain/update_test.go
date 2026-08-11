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

// TestUpdateCommand_RunE exercises the real updateCmd.RunE (not a re-implementation of its
// logic), covering the flag-binding -> dry-run resolution -> max-concurrency resolution ->
// toolchain.RunUpdate wiring added in cmd/toolchain/update.go. It points VersionsFile at a
// non-existent path in an isolated temp dir so toolchain.RunUpdate's LoadToolVersions call fails
// deterministically instead of touching any real repo state, letting us assert the cmd layer
// propagates that error unwrapped.
func TestUpdateCommand_RunE(t *testing.T) {
	tempDir := t.TempDir()
	toolchain.SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: filepath.Join(tempDir, "tool-versions-does-not-exist"),
		},
	})
	t.Cleanup(func() { toolchain.SetAtmosConfig(nil) })

	tests := []struct {
		name           string
		dryRun         string
		maxConcurrency string
		wantErr        error
	}{
		{
			name:    "default flags reach RunUpdate and propagate its load failure",
			wantErr: errUtils.ErrToolVersionsFileOperation,
		},
		{
			name:    "dry-run flag is read without breaking the wiring",
			dryRun:  "true",
			wantErr: errUtils.ErrToolVersionsFileOperation,
		},
		{
			name:           "invalid max-concurrency is rejected before RunUpdate is ever called",
			maxConcurrency: "0",
			wantErr:        errUtils.ErrInvalidFlagValue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() {
				require.NoError(t, updateCmd.Flags().Set("dry-run", "false"))
				require.NoError(t, updateCmd.Flags().Set("max-concurrency", "0"))
				// pflag.Set marks the flag Changed regardless of the value passed, and
				// resolveInstallMaxConcurrency/IsBoolFlagExplicitlySet key off Changed
				// (not just the value) to decide whether a flag was explicitly passed.
				// Reset it directly so the next subtest starts from a truly-unset flag,
				// not one that merely carries the default value.
				updateCmd.Flags().Lookup("dry-run").Changed = false
				updateCmd.Flags().Lookup("max-concurrency").Changed = false
				viper.Reset()
			})

			if tt.dryRun != "" {
				require.NoError(t, updateCmd.Flags().Set("dry-run", tt.dryRun))
			}
			if tt.maxConcurrency != "" {
				require.NoError(t, updateCmd.Flags().Set("max-concurrency", tt.maxConcurrency))
			}

			err := updateCmd.RunE(updateCmd, []string{})

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
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
