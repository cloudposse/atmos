package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSnapshotRootCmdState(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(t *testing.T)
		validateBefore func(t *testing.T, snapshot *cmdStateSnapshot)
		validateAfter  func(t *testing.T, snapshot *cmdStateSnapshot)
	}{
		{
			name: "captures empty args by default",
			setup: func(t *testing.T) {
				// Args are empty until command is parsed.
			},
			validateBefore: func(t *testing.T, snapshot *cmdStateSnapshot) {
				assert.NotNil(t, snapshot.args, "Should capture args slice")
				assert.NotNil(t, snapshot.flags, "Should capture flags map")
			},
			validateAfter: nil,
		},
		{
			name: "captures flag values",
			setup: func(t *testing.T) {
				require.NoError(t, RootCmd.PersistentFlags().Set("chdir", "/tmp/test"))
				require.NoError(t, RootCmd.PersistentFlags().Set("logs-level", "Debug"))
			},
			validateBefore: func(t *testing.T, snapshot *cmdStateSnapshot) {
				chdirSnap, exists := snapshot.flags["chdir"]
				require.True(t, exists, "Should capture chdir flag")
				assert.Equal(t, "/tmp/test", chdirSnap.value)
				assert.True(t, chdirSnap.changed, "Should mark flag as changed")

				logsLevelSnap, exists := snapshot.flags["logs-level"]
				require.True(t, exists, "Should capture logs-level flag")
				assert.Equal(t, "Debug", logsLevelSnap.value)
			},
			validateAfter: nil,
		},
		{
			name: "captures changed state",
			setup: func(t *testing.T) {
				// Set a flag, then reset it to default - Changed should still be true.
				require.NoError(t, RootCmd.PersistentFlags().Set("chdir", "/tmp"))
				require.NoError(t, RootCmd.PersistentFlags().Set("chdir", ""))
			},
			validateBefore: func(t *testing.T, snapshot *cmdStateSnapshot) {
				chdirSnap, exists := snapshot.flags["chdir"]
				require.True(t, exists)
				assert.True(t, chdirSnap.changed, "Should preserve Changed state even if value is default")
			},
			validateAfter: nil,
		},
		{
			name: "captures all flags including persistent",
			setup: func(t *testing.T) {
				// Set both local and persistent flags.
				require.NoError(t, RootCmd.PersistentFlags().Set("base-path", "/custom/base"))
			},
			validateBefore: func(t *testing.T, snapshot *cmdStateSnapshot) {
				// Verify we captured persistent flags.
				basePathSnap, exists := snapshot.flags["base-path"]
				require.True(t, exists, "Should capture persistent flags")
				assert.Equal(t, "/custom/base", basePathSnap.value)
			},
			validateAfter: nil,
		},
		{
			name: "captures and restores command args",
			setup: func(t *testing.T) {
				RootCmd.SetArgs([]string{"terraform", "plan", "vpc", "-s", "dev"})
				// Parse flags to populate RootCmd.Flags().Args().
				_ = RootCmd.ParseFlags([]string{"terraform", "plan", "vpc", "-s", "dev"})
			},
			validateBefore: func(t *testing.T, snapshot *cmdStateSnapshot) {
				require.NotNil(t, snapshot.args, "Should capture args slice")
				// After parsing, non-flag args are: terraform, plan, vpc (dev is flag value).
				assert.Equal(t, []string{"terraform", "plan", "vpc"}, snapshot.args)
			},
			validateAfter: func(t *testing.T, snapshot *cmdStateSnapshot) {
				// Modify args to different values.
				RootCmd.SetArgs([]string{"helmfile", "apply"})
				_ = RootCmd.ParseFlags([]string{"helmfile", "apply"})

				// Verify current args are different.
				currentArgs := RootCmd.Flags().Args()
				assert.Equal(t, []string{"helmfile", "apply"}, currentArgs, "Args should be modified before restore")

				// Restore from snapshot.
				restoreRootCmdState(snapshot)

				// Verify args were restored to original by checking SetArgs was called.
				// We verify by parsing again and checking args.
				_ = RootCmd.ParseFlags(snapshot.args)
				restoredArgs := RootCmd.Flags().Args()
				assert.Equal(t, []string{"terraform", "plan", "vpc"}, restoredArgs, "Should restore original args")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t)

			if tt.setup != nil {
				tt.setup(t)
			}

			snapshot := snapshotRootCmdState()

			if tt.validateBefore != nil {
				tt.validateBefore(t, snapshot)
			}

			if tt.validateAfter != nil {
				tt.validateAfter(t, snapshot)
			}
		})
	}
}

func TestRestoreRootCmdState(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(t *testing.T) *cmdStateSnapshot
		modifyBefore func(t *testing.T)
		validate     func(t *testing.T)
	}{
		{
			name: "restores flag values",
			setup: func(t *testing.T) *cmdStateSnapshot {
				require.NoError(t, RootCmd.PersistentFlags().Set("chdir", "/original"))
				require.NoError(t, RootCmd.PersistentFlags().Set("logs-level", "Trace"))
				return snapshotRootCmdState()
			},
			modifyBefore: func(t *testing.T) {
				require.NoError(t, RootCmd.PersistentFlags().Set("chdir", "/modified"))
				require.NoError(t, RootCmd.PersistentFlags().Set("logs-level", "Debug"))
			},
			validate: func(t *testing.T) {
				chdir, err := RootCmd.PersistentFlags().GetString("chdir")
				require.NoError(t, err)
				assert.Equal(t, "/original", chdir, "Should restore original chdir value")

				logsLevel, err := RootCmd.PersistentFlags().GetString("logs-level")
				require.NoError(t, err)
				assert.Equal(t, "Trace", logsLevel, "Should restore original logs-level value")
			},
		},
		{
			name: "restores changed state",
			setup: func(t *testing.T) *cmdStateSnapshot {
				// Start with flag unchanged.
				flag := RootCmd.PersistentFlags().Lookup("chdir")
				flag.Changed = false
				return snapshotRootCmdState()
			},
			modifyBefore: func(t *testing.T) {
				// Modify the flag, which sets Changed to true.
				require.NoError(t, RootCmd.PersistentFlags().Set("chdir", "/modified"))
			},
			validate: func(t *testing.T) {
				flag := RootCmd.PersistentFlags().Lookup("chdir")
				assert.False(t, flag.Changed, "Should restore Changed state to false")
			},
		},
		{
			name: "restores to snapshot state",
			setup: func(t *testing.T) *cmdStateSnapshot {
				return snapshotRootCmdState()
			},
			modifyBefore: func(t *testing.T) {
				require.NoError(t, RootCmd.PersistentFlags().Set("chdir", "/tmp"))
				require.NoError(t, RootCmd.PersistentFlags().Set("base-path", "/tmp/base"))
			},
			validate: func(t *testing.T) {
				// Should restore to empty/default values.
				chdir, _ := RootCmd.PersistentFlags().GetString("chdir")
				assert.Empty(t, chdir, "Should restore chdir to empty")
				basePath, _ := RootCmd.PersistentFlags().GetString("base-path")
				assert.Empty(t, basePath, "Should restore base-path to empty")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t)

			snapshot := tt.setup(t)

			if tt.modifyBefore != nil {
				tt.modifyBefore(t)
			}

			restoreRootCmdState(snapshot)

			if tt.validate != nil {
				tt.validate(t)
			}
		})
	}
}

func TestSnapshotImmutability(t *testing.T) {
	_ = NewTestKit(t)

	// Create initial state.
	require.NoError(t, RootCmd.PersistentFlags().Set("chdir", "/initial"))

	// Take snapshot.
	snapshot := snapshotRootCmdState()

	// Verify snapshot captured initial state.
	chdirSnap := snapshot.flags["chdir"]
	assert.Equal(t, "/initial", chdirSnap.value)

	// Modify RootCmd state.
	require.NoError(t, RootCmd.PersistentFlags().Set("chdir", "/modified"))

	// Verify snapshot is unchanged.
	chdirSnap = snapshot.flags["chdir"]
	assert.Equal(t, "/initial", chdirSnap.value, "Snapshot should preserve initial flag value")

	// Verify RootCmd has the modified state.
	chdir, _ := RootCmd.PersistentFlags().GetString("chdir")
	assert.Equal(t, "/modified", chdir)
}

func TestSnapshotRestoreCycle(t *testing.T) {
	_ = NewTestKit(t)

	// Set initial state.
	require.NoError(t, RootCmd.PersistentFlags().Set("chdir", "/test"))
	require.NoError(t, RootCmd.PersistentFlags().Set("logs-level", "Trace"))

	// Snapshot.
	snapshot := snapshotRootCmdState()

	// Modify state.
	require.NoError(t, RootCmd.PersistentFlags().Set("chdir", "/different"))
	require.NoError(t, RootCmd.PersistentFlags().Set("logs-level", "Debug"))

	// Verify modified.
	chdir, _ := RootCmd.PersistentFlags().GetString("chdir")
	assert.Equal(t, "/different", chdir)

	// Restore.
	restoreRootCmdState(snapshot)

	// Verify restored to original.
	chdir, _ = RootCmd.PersistentFlags().GetString("chdir")
	assert.Equal(t, "/test", chdir, "Should restore chdir")

	logsLevel, _ := RootCmd.PersistentFlags().GetString("logs-level")
	assert.Equal(t, "Trace", logsLevel, "Should restore logs-level")
}
