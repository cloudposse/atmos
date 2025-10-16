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

// WithRootCmdSnapshot ensures RootCmd state is captured before the test
// and restored in cleanup, providing complete test isolation without maintaining
// a hardcoded list of flags to reset.
//
// This snapshots ALL flag values and their Changed state, then restores them
// exactly in cleanup. This prevents test pollution without needing to know
// which flags exist or what their defaults are.
//
// Usage:
//
//	func TestExample(t *testing.T) {
//	    defer WithRootCmdSnapshot(t)()
//
//	    // Your test code here - any RootCmd state changes will be
//	    // automatically reverted when the test completes.
//	}
//
// Why this approach:
//   - No hardcoded flag lists to maintain
//   - Works for any flags (current and future)
//   - Restores exact state, not just defaults
//   - Eliminates entire class of test pollution problems
func WithRootCmdSnapshot(t *testing.T) func() {
	t.Helper()
	snapshot := snapshotRootCmdState()
	return func() {
		restoreRootCmdState(snapshot)
	}
}
