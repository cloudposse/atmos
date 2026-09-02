package cmd

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKit wraps testing.TB and provides automatic RootCmd state cleanup.
// It follows Go 1.15+ testing.TB interface pattern for composable test helpers.
//
// Usage:
//
//	func TestMyCommand(t *testing.T) {
//	    t := NewTestKit(t)
//	    // RootCmd state automatically cleaned up after test
//	    // ... test code ...
//	}
type TestKit struct {
	testing.TB
}

// NewTestKit creates a TestKit that wraps testing.TB and automatically registers
// RootCmd state cleanup. This follows the testing.TB interface pattern introduced
// in Go 1.15+ for composable test helpers.
//
// The TestKit automatically:
// - Snapshots RootCmd state when created
// - Registers cleanup to restore state when test completes
// - Works with subtests and table-driven tests
// - Prevents test pollution from global RootCmd state
//
// Example:
//
//	func TestCommand(t *testing.T) {
//	    t := NewTestKit(t)
//	    // Your test code - RootCmd cleanup is automatic
//	    t.Setenv("FOO", "bar") // All testing.TB methods work
//	}
//
// Table-driven tests:
//
//	func TestTableDriven(t *testing.T) {
//	    t := NewTestKit(t) // Parent gets cleanup
//	    tests := []struct{...}{...}
//	    for _, tt := range tests {
//	        t.Run(tt.name, func(t *testing.T) {
//	            t := NewTestKit(t) // Each subtest gets cleanup
//	            // Test code...
//	        })
//	    }
//	}
func NewTestKit(tb testing.TB) *TestKit {
	tb.Helper()

	// Snapshot RootCmd state and register cleanup.
	snapshot := snapshotRootCmdState()
	tb.Cleanup(func() {
		restoreRootCmdState(snapshot)
	})

	return &TestKit{TB: tb}
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

// rootCmdCommandNames returns the Name() of every command currently registered on RootCmd.
func rootCmdCommandNames(tb testing.TB) []string {
	tb.Helper()
	names := make([]string, 0, len(RootCmd.Commands()))
	for _, c := range RootCmd.Commands() {
		names = append(names, c.Name())
	}
	return names
}

// TestTestKit_RootCmdCommandsRestoration verifies that a command registered on RootCmd during a
// test (e.g. by processCustomCommands) is removed again once the test's cleanup runs, while
// commands that were already on RootCmd before the test remain untouched.
func TestTestKit_RootCmdCommandsRestoration(t *testing.T) {
	tests := []struct {
		name        string
		addCommands []string
	}{
		{name: "single command added", addCommands: []string{"testkit-added-one"}},
		{name: "multiple commands added", addCommands: []string{"testkit-added-two", "testkit-added-three"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalNames := rootCmdCommandNames(t)

			t.Run("adds commands", func(t *testing.T) {
				tk := NewTestKit(t)

				for _, name := range tt.addCommands {
					RootCmd.AddCommand(&cobra.Command{Use: name})
				}

				// Sanity check: the added commands are actually present mid-test.
				midTestNames := rootCmdCommandNames(tk)
				for _, name := range tt.addCommands {
					assert.Contains(tk, midTestNames, name, "added command should be present mid-test")
				}
				// Cleanup happens automatically when this subtest ends.
			})

			// After cleanup, added commands must be gone and originals must remain.
			restoredNames := rootCmdCommandNames(t)
			for _, name := range tt.addCommands {
				assert.NotContains(t, restoredNames, name, "command added during the test should be removed by restore")
			}
			assert.ElementsMatch(t, originalNames, restoredNames, "original commands should be unchanged after restore")
		})
	}
}

// TestTestKit_RootCmdCommandRemovalRestoration verifies that a command present in a test's
// RootCmd snapshot is re-added if that test (or a nested one) removes it via
// RootCmd.RemoveCommand. The restoreRootCmdCommands helper previously only removed commands
// *added* after the snapshot; a snapshot command removed mid-test stayed gone for every later
// test.
func TestTestKit_RootCmdCommandRemovalRestoration(t *testing.T) {
	tk := NewTestKit(t) // Outer snapshot includes the command registered below.

	original := &cobra.Command{Use: "testkit-removal-target"}
	RootCmd.AddCommand(original)
	require.Contains(tk, rootCmdCommandNames(tk), "testkit-removal-target")

	t.Run("nested test removes an original command", func(t *testing.T) {
		innerTk := NewTestKit(t) // Inner snapshot also includes "testkit-removal-target".

		RootCmd.RemoveCommand(original)

		assert.NotContains(innerTk, rootCmdCommandNames(innerTk), "testkit-removal-target",
			"command should be removed mid-test")
		// Cleanup happens automatically when this subtest ends.
	})

	// The nested test's cleanup must restore the command it removed (present in its own
	// snapshot), not leave it gone for the rest of the outer test.
	assert.Contains(t, rootCmdCommandNames(t), "testkit-removal-target",
		"a command removed inside a nested NewTestKit test must be restored by that test's cleanup")
}

// TestTestKit_GrandchildCommandRestoration verifies that NewTestKit restores a
// subcommand added under (or removed from) a *non-RootCmd* parent, not just
// direct RootCmd children. The restoreCommandChildren helper walks the whole
// snapshotted tree recursively; this is the regression test for the case a
// flat, RootCmd-only restore would miss.
//
// Uses a throwaway parent command registered on RootCmd for the duration of
// the test, rather than mutating a real subcommand's (e.g. "toolchain")
// actual children: real commands are shared with other tests that drive
// them through RootCmd.ExecuteC() (see TestRootHelpFunc_RealTree_*), and
// even correctly-restored churn on their child list is enough to make those
// tests' own shuffle-order assumptions flaky.
func TestTestKit_GrandchildCommandRestoration(t *testing.T) {
	tk := NewTestKit(t)

	parent := &cobra.Command{Use: "testkit-grandchild-parent"}
	original := &cobra.Command{Use: "testkit-original-grandchild"}
	parent.AddCommand(original)
	RootCmd.AddCommand(parent)
	require.NotNil(tk, findChildCommand(parent, "testkit-original-grandchild"),
		"fixture parent should start with its one original grandchild")

	t.Run("adds and removes a grandchild command", func(t *testing.T) {
		innerTk := NewTestKit(t)

		added := &cobra.Command{Use: "testkit-added-grandchild"}
		parent.AddCommand(added)
		require.NotNil(innerTk, findChildCommand(parent, "testkit-added-grandchild"),
			"added grandchild should be present mid-test")

		parent.RemoveCommand(original)
		assert.Nil(innerTk, findChildCommand(parent, "testkit-original-grandchild"),
			"removed grandchild should be absent mid-test")
		// Cleanup happens automatically when this subtest ends.
	})

	// After cleanup, the added grandchild must be gone and the original
	// grandchild (removed mid-test) must be back.
	assert.Nil(t, findChildCommand(parent, "testkit-added-grandchild"),
		"grandchild added during the test should be removed by restore")
	assert.NotNil(t, findChildCommand(parent, "testkit-original-grandchild"),
		"grandchild removed during the test should be restored")
}

// Note: Viper restoration tests were removed because viper.Set(key, nil) breaks BindPFlag connections.
// Viper state isolation between tests requires a different approach (e.g., temporary viper instances)
// which is out of scope for the current TestKit implementation. Tests that need viper isolation
// should use explicit cleanup as demonstrated in auth_login_test.go.

// TestTestKit_RestoresAtmosConfig covers the package-level atmosConfig
// snapshot: a test that flips a field on the global (as any real
// Execute()/InitCliConfig() path does when it detects a CI provider) must not
// leak that value into the next test.
func TestTestKit_RestoresAtmosConfig(t *testing.T) {
	_ = NewTestKit(t)
	initial := atmosConfig.CI.Enabled

	t.Run("mutates atmosConfig", func(t *testing.T) {
		_ = NewTestKit(t)
		atmosConfig.CI.Enabled = !initial
		assert.Equal(t, !initial, atmosConfig.CI.Enabled)
	})

	assert.Equal(t, initial, atmosConfig.CI.Enabled, "atmosConfig should be restored after subtest")
}

// TestResetFlagToDefault covers the bool DefValue trap: cmd/root.go blanks the
// --version flag's DefValue to "" for cleaner help output, and pflag's bool
// Set("") fails, so a naive Set(DefValue) would leave a leaked true in place.
func TestResetFlagToDefault(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	fs.Bool("version", false, "")
	fs.String("stack", "", "")
	fs.StringSlice("skip", nil, "")

	versionFlag := fs.Lookup("version")
	versionFlag.DefValue = "" // Mirror cmd/root.go's cosmetic blanking.
	require.NoError(t, fs.Set("version", "true"))
	require.NoError(t, fs.Set("stack", "prod"))
	require.NoError(t, fs.Set("skip", "a,b"))

	fs.VisitAll(resetFlagToDefault)

	assert.Equal(t, "false", versionFlag.Value.String())
	assert.False(t, versionFlag.Changed)
	assert.Equal(t, "", fs.Lookup("stack").Value.String())
	assert.False(t, fs.Lookup("stack").Changed)
	assert.False(t, fs.Lookup("skip").Changed)
}
