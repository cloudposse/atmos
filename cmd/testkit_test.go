package cmd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tests/testhelpers"
)

// TestKit is an alias for testhelpers.TestKit to maintain backward compatibility.
type TestKit = testhelpers.TestKit

// NewTestKit creates a TestKit for RootCmd. This is a convenience wrapper that
// automatically passes RootCmd to testhelpers.NewTestKit for cmd package tests.
//
// Usage:
//
//	func TestMyCommand(t *testing.T) {
//	    t := NewTestKit(t)
//	    // RootCmd state automatically cleaned up after test
//	    // ... test code ...
//	}
func NewTestKit(tb testing.TB) *TestKit {
	tb.Helper()
	return testhelpers.NewTestKit(tb, RootCmd)
}

func TestTestKit_AutomaticCleanup(t *testing.T) {
	// Capture initial state.
	initialChdir, _ := RootCmd.PersistentFlags().GetString("chdir")

	// Run test that modifies RootCmd.
	t.Run("modifies RootCmd", func(t *testing.T) {
		tk := NewTestKit(t)

		// Modify RootCmd state.
		require.NoError(tk, RootCmd.PersistentFlags().Set("chdir", "/modified"))
		chdir, _ := RootCmd.PersistentFlags().GetString("chdir")
		assert.Equal(tk, "/modified", chdir)
		// Cleanup happens automatically when subtest ends.
	})

	// Verify state was restored after subtest.
	chdir, _ := RootCmd.PersistentFlags().GetString("chdir")
	assert.Equal(t, initialChdir, chdir, "RootCmd state should be restored after subtest")
}

func TestTestKit_ImplementsTestingTB(t *testing.T) {
	tk := NewTestKit(t)

	// Verify TestKit implements testing.TB interface.
	var _ testing.TB = tk

	// Test that TB methods work.
	tk.Helper()
	tk.Log("TestKit implements testing.TB")
	tk.Setenv("TESTKIT_TEST", "value")
	// Verify environment variable was set.
	assert.Equal(tk, "value", os.Getenv("TESTKIT_TEST"))
}

func TestTestKit_TableDrivenTests(t *testing.T) {
	_ = NewTestKit(t) // Parent test gets cleanup.

	tests := []struct {
		name     string
		chdir    string
		expected string
	}{
		{
			name:     "set chdir to /tmp",
			chdir:    "/tmp",
			expected: "/tmp",
		},
		{
			name:     "set chdir to /var",
			chdir:    "/var",
			expected: "/var",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tk := NewTestKit(t) // Each subtest gets its own cleanup.

			require.NoError(tk, RootCmd.PersistentFlags().Set("chdir", tt.chdir))
			chdir, _ := RootCmd.PersistentFlags().GetString("chdir")
			assert.Equal(tk, tt.expected, chdir)
			// Cleanup happens automatically for this subtest.
		})
	}

	// After all subtests, parent cleanup ensures no pollution.
}

func TestTestKit_NestedTests(t *testing.T) {
	_ = NewTestKit(t)

	t.Run("level1", func(t *testing.T) {
		tk := NewTestKit(t)
		require.NoError(tk, RootCmd.PersistentFlags().Set("chdir", "/level1"))

		t.Run("level2", func(t *testing.T) {
			tk := NewTestKit(t)
			require.NoError(tk, RootCmd.PersistentFlags().Set("chdir", "/level2"))

			chdir, _ := RootCmd.PersistentFlags().GetString("chdir")
			assert.Equal(tk, "/level2", chdir)
			// level2 cleanup.
		})

		// After level2, should be restored to level1.
		chdir, _ := RootCmd.PersistentFlags().GetString("chdir")
		assert.Equal(tk, "/level1", chdir)
		// level1 cleanup.
	})

	// After all nested tests, state fully restored.
}

func TestTestKit_OsArgsRestoration(t *testing.T) {
	// Capture initial os.Args.
	initialArgs := make([]string, len(os.Args))
	copy(initialArgs, os.Args)

	// Run test that modifies os.Args.
	t.Run("modifies os.Args", func(t *testing.T) {
		_ = NewTestKit(t)

		// Modify os.Args.
		os.Args = []string{"atmos", "test", "modified"}
		assert.Equal(t, []string{"atmos", "test", "modified"}, os.Args)
		// Cleanup happens automatically when subtest ends.
	})

	// Verify os.Args was restored after subtest.
	assert.Equal(t, initialArgs, os.Args, "os.Args should be restored after subtest")
}
