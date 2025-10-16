package cmd

import (
	"testing"

	"github.com/spf13/pflag"
)

// flagSnapshot stores the state of a flag for restoration.
type flagSnapshot struct {
	value   string
	changed bool
}

// cmdStateSnapshot stores the complete state of RootCmd for restoration.
type cmdStateSnapshot struct {
	args  []string
	flags map[string]flagSnapshot
}

// snapshotRootCmdState captures the current state of RootCmd including all flag values.
// This allows tests to save state at the beginning and restore it in cleanup,
// preventing test pollution without needing to maintain a hardcoded list of flags.
func snapshotRootCmdState() *cmdStateSnapshot {
	snapshot := &cmdStateSnapshot{
		args:  make([]string, len(RootCmd.Flags().Args())),
		flags: make(map[string]flagSnapshot),
	}

	// Copy args.
	copy(snapshot.args, RootCmd.Flags().Args())

	// Snapshot all flags (both local and persistent).
	snapshotFlags := func(flagSet *pflag.FlagSet) {
		flagSet.VisitAll(func(f *pflag.Flag) {
			snapshot.flags[f.Name] = flagSnapshot{
				value:   f.Value.String(),
				changed: f.Changed,
			}
		})
	}

	snapshotFlags(RootCmd.Flags())
	snapshotFlags(RootCmd.PersistentFlags())

	return snapshot
}

// restoreRootCmdState restores RootCmd to a previously captured state.
func restoreRootCmdState(snapshot *cmdStateSnapshot) {
	// Clear command args.
	RootCmd.SetArgs([]string{})

	// Restore all flags to their snapshotted values.
	restoreFlags := func(flagSet *pflag.FlagSet) {
		flagSet.VisitAll(func(f *pflag.Flag) {
			if snap, ok := snapshot.flags[f.Name]; ok {
				_ = f.Value.Set(snap.value)
				f.Changed = snap.changed
			}
		})
	}

	restoreFlags(RootCmd.Flags())
	restoreFlags(RootCmd.PersistentFlags())
}

// resetRootCmdForTesting resets the global RootCmd state to prevent test pollution.
//
// Deprecated: Use WithRootCmdSnapshot instead for automatic state management.
//
// IMPORTANT: RootCmd is a global package variable. When tests call RootCmd.SetArgs()
// and RootCmd.ParseFlags(), the flag values persist across tests. Simply calling
// RootCmd.SetArgs([]string{}) does NOT clear already-parsed flag values!
//
// This function must be called in test cleanup to ensure proper test isolation:
//
//	t.Cleanup(func() {
//	    resetRootCmdForTesting(t)
//	})
func resetRootCmdForTesting(t *testing.T) {
	t.Helper()

	// Clear command args.
	RootCmd.SetArgs([]string{})

	// Clear all persistent flag values that might have been set by previous tests.
	// We must explicitly reset flag values because SetArgs() alone doesn't clear them.
	// NOTE: We only reset flags that tests commonly set for chdir functionality.
	// We explicitly DO NOT reset config-related flags (config, config-path) as
	// resetting them to empty values breaks config loading in subsequent tests.
	flags := []string{
		"chdir",
		"base-path",
		"stacks-dir",
		"components-dir",
		"workflows-dir",
		"logs-file",
		"logs-level",
	}

	for _, flag := range flags {
		// Lookup in both local and persistent flag sets.
		f := RootCmd.Flags().Lookup(flag)
		if f == nil {
			f = RootCmd.PersistentFlags().Lookup(flag)
		}
		if f != nil {
			// Reset to default and clear Changed.
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		}
	}
}

// WithRootCmdSnapshot ensures RootCmd state is captured before the test
// and restored in cleanup, providing complete test isolation without maintaining
// a hardcoded list of flags to reset.
//
// Usage:
//
//	func TestExample(t *testing.T) {
//	    defer WithRootCmdSnapshot(t)()
//
//	    // Your test code here - any RootCmd state changes will be
//	    // automatically reverted when the test completes.
//	}
func WithRootCmdSnapshot(t *testing.T) func() {
	t.Helper()
	snapshot := snapshotRootCmdState()
	return func() {
		restoreRootCmdState(snapshot)
	}
}
