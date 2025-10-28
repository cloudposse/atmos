package testhelpers

import (
	"os"
	"reflect"
	"testing"

	"github.com/spf13/pflag"

	"github.com/cloudposse/atmos/cmd"
)

// RootCmdTestKit wraps testing.TB and provides automatic RootCmd state cleanup.
// It follows Go 1.15+ testing.TB interface pattern for composable test helpers.
//
// Usage:
//
//	func TestMyCommand(t *testing.T) {
//	    t := testhelpers.NewRootCmdTestKit(t)
//	    // RootCmd state automatically cleaned up after test
//	    // ... test code ...
//	}
type RootCmdTestKit struct {
	testing.TB
}

// NewRootCmdTestKit creates a RootCmdTestKit that wraps testing.TB and automatically registers
// RootCmd state cleanup. This follows the testing.TB interface pattern introduced
// in Go 1.15+ for composable test helpers.
//
// The RootCmdTestKit automatically:
// - Snapshots RootCmd state when created
// - Registers cleanup to restore state when test completes
// - Works with subtests and table-driven tests
// - Prevents test pollution from global RootCmd state
//
// Example:
//
//	func TestCommand(t *testing.T) {
//	    t := testhelpers.NewRootCmdTestKit(t)
//	    // Your test code - RootCmd cleanup is automatic
//	    t.Setenv("FOO", "bar") // All testing.TB methods work
//	}
//
// Table-driven tests:
//
//	func TestTableDriven(t *testing.T) {
//	    t := testhelpers.NewRootCmdTestKit(t) // Parent gets cleanup
//	    tests := []struct{...}{...}
//	    for _, tt := range tests {
//	        t.Run(tt.name, func(t *testing.T) {
//	            t := testhelpers.NewRootCmdTestKit(t) // Each subtest gets cleanup
//	            // Test code...
//	        })
//	    }
//	}
func NewRootCmdTestKit(tb testing.TB) *RootCmdTestKit {
	tb.Helper()

	// Snapshot RootCmd state and register cleanup.
	snapshot := snapshotRootCmdState()
	tb.Cleanup(func() {
		restoreRootCmdState(snapshot)
	})

	return &RootCmdTestKit{TB: tb}
}

// flagSnapshot stores the state of a flag for restoration.
type flagSnapshot struct {
	value   string
	changed bool
}

// cmdStateSnapshot stores the complete state of RootCmd for restoration.
type cmdStateSnapshot struct {
	args   []string
	osArgs []string
	flags  map[string]flagSnapshot
}

// snapshotRootCmdState captures the current state of RootCmd including all flag values.
// This allows tests to save state at the beginning and restore it in cleanup via NewRootCmdTestKit,
// preventing test pollution without needing to maintain a hardcoded list of flags.
func snapshotRootCmdState() *cmdStateSnapshot {
	snapshot := &cmdStateSnapshot{
		args:   make([]string, len(cmd.RootCmd.Flags().Args())),
		osArgs: make([]string, len(os.Args)),
		flags:  make(map[string]flagSnapshot),
	}

	// Copy args.
	copy(snapshot.args, cmd.RootCmd.Flags().Args())

	// Copy os.Args.
	copy(snapshot.osArgs, os.Args)

	// Snapshot all flags (both local and persistent).
	snapshotFlags := func(flagSet *pflag.FlagSet) {
		flagSet.VisitAll(func(f *pflag.Flag) {
			snapshot.flags[f.Name] = flagSnapshot{
				value:   f.Value.String(),
				changed: f.Changed,
			}
		})
	}

	snapshotFlags(cmd.RootCmd.Flags())
	snapshotFlags(cmd.RootCmd.PersistentFlags())

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
	// Restore command args.
	cmd.RootCmd.SetArgs(snapshot.args)

	// Restore os.Args.
	os.Args = make([]string, len(snapshot.osArgs))
	copy(os.Args, snapshot.osArgs)

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

	restoreFlags(cmd.RootCmd.Flags())
	restoreFlags(cmd.RootCmd.PersistentFlags())
}
