package cmd

import (
	"reflect"
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

// restoreStringSliceFlag handles restoration of StringSlice/StringArray flags.
// These flag types have Set() methods that append rather than replace, so we need
// to use reflection to clear the underlying slice first.
func restoreStringSliceFlag(f *pflag.Flag, snap flagSnapshot) {
	// Use reflection to access the underlying slice and clear it.
	v := reflect.ValueOf(f.Value)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	// Look for a field that holds the slice (usually "value").
	if v.Kind() == reflect.Struct {
		valueField := v.FieldByName("value")
		if valueField.IsValid() && valueField.CanSet() {
			// Reset to empty slice to prevent append behavior.
			valueField.Set(reflect.MakeSlice(valueField.Type(), 0, 0))
		}
	}
	// Reset Changed state before setting value.
	f.Changed = false
	// Set the snapshot value if not default.
	if snap.value != "[]" && snap.value != "" {
		_ = f.Value.Set(snap.value)
	}
	// Restore Changed state.
	f.Changed = snap.changed
}

// restoreRootCmdState restores RootCmd to a previously captured state.
func restoreRootCmdState(snapshot *cmdStateSnapshot) {
	// Clear command args.
	RootCmd.SetArgs([]string{})

	// Restore all flags to their snapshotted values.
	restoreFlags := func(flagSet *pflag.FlagSet) {
		flagSet.VisitAll(func(f *pflag.Flag) {
			if snap, ok := snapshot.flags[f.Name]; ok {
				// StringSlice/StringArray flags need special handling due to append behavior.
				if f.Value.Type() == "stringSlice" || f.Value.Type() == "stringArray" {
					restoreStringSliceFlag(f, snap)
					return
				}
				// For other flag types, direct Set() works fine.
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
// Usage (defer pattern):
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

// CleanupRootCmd registers RootCmd state cleanup with t.Cleanup(), similar to t.Setenv().
// This provides an ergonomic API that automatically snapshots and restores RootCmd state
// when the test completes.
//
// USAGE - Call this at the START of EVERY test that uses RootCmd:
//
//	func TestExample(t *testing.T) {
//	    CleanupRootCmd(t) // Single line at the start - no defer needed!
//
//	    // Modify RootCmd state as needed.
//	    RootCmd.SetArgs([]string{"terraform", "plan"})
//	    RootCmd.PersistentFlags().Set("chdir", "/tmp")
//
//	    // State automatically restored when test completes.
//	}
//
// WHY THIS IS REQUIRED:
//   - RootCmd is global state shared across all tests
//   - Without cleanup, flag values persist causing mysterious test failures
//   - Tests pass in isolation but fail when run together
//   - StringSlice flags (config, config-path) are especially problematic
//
// WHEN TO USE:
//   - ANY test that calls RootCmd.Execute(), RootCmd.SetArgs(), or modifies RootCmd flags
//   - ANY test that calls commands which internally use RootCmd
//   - When in doubt, add it - it's safe and prevents hard-to-debug issues
//
// This is the RECOMMENDED pattern for all cmd/ tests. It's more ergonomic than the defer
// pattern and follows Go's testing conventions (similar to t.Setenv, t.Chdir, etc).
func CleanupRootCmd(t *testing.T) {
	t.Helper()
	snapshot := snapshotRootCmdState()
	t.Cleanup(func() {
		restoreRootCmdState(snapshot)
	})
}
