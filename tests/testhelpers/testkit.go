package testhelpers

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestKit wraps testing.TB and provides automatic cobra.Command state cleanup.
// It follows Go 1.15+ testing.TB interface pattern for composable test helpers.
//
// Usage:
//
//	func TestMyCommand(t *testing.T) {
//	    t := NewTestKit(t, myCmd)
//	    // Command state automatically cleaned up after test
//	    // ... test code ...
//	}
type TestKit struct {
	testing.TB
}

// NewTestKit creates a TestKit that wraps testing.TB and automatically registers
// cobra.Command state cleanup. This follows the testing.TB interface pattern introduced
// in Go 1.15+ for composable test helpers.
//
// The TestKit automatically:
// - Snapshots command state when created
// - Registers cleanup to restore state when test completes
// - Works with subtests and table-driven tests
// - Prevents test pollution from global command state
//
// Parameters:
//   - tb: testing.TB instance (usually *testing.T)
//   - cmd: cobra.Command to snapshot and restore (e.g., RootCmd)
//
// Example:
//
//	func TestCommand(t *testing.T) {
//	    t := NewTestKit(t, RootCmd)
//	    // Your test code - command cleanup is automatic
//	    t.Setenv("FOO", "bar") // All testing.TB methods work
//	}
//
// Table-driven tests:
//
//	func TestTableDriven(t *testing.T) {
//	    t := NewTestKit(t, RootCmd) // Parent gets cleanup
//	    tests := []struct{...}{...}
//	    for _, tt := range tests {
//	        t.Run(tt.name, func(t *testing.T) {
//	            t := NewTestKit(t, RootCmd) // Each subtest gets cleanup
//	            // Test code...
//	        })
//	    }
//	}
func NewTestKit(tb testing.TB, cmd *cobra.Command) *TestKit {
	tb.Helper()

	// Snapshot command state and register cleanup using shared cobra helpers.
	snapshot := SnapshotCobraState(cmd)
	tb.Cleanup(func() {
		RestoreCobraState(cmd, snapshot)
	})

	return &TestKit{TB: tb}
}
