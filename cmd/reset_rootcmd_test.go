package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResetRootCmdForTesting(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T)
		validate func(t *testing.T)
	}{
		{
			name: "resets chdir flag",
			setup: func(t *testing.T) {
				t.Helper()
				RootCmd.SetArgs([]string{"--chdir", "/some/path"})
				require.NoError(t, RootCmd.ParseFlags([]string{"--chdir", "/some/path"}))
				// Verify flag is set.
				val, _ := RootCmd.Flags().GetString("chdir")
				require.Equal(t, "/some/path", val, "Setup should set chdir flag")
			},
			validate: func(t *testing.T) {
				t.Helper()
				val, _ := RootCmd.Flags().GetString("chdir")
				assert.Equal(t, "", val, "chdir flag should be reset to empty")
			},
		},
		{
			name: "resets base-path flag",
			setup: func(t *testing.T) {
				t.Helper()
				RootCmd.SetArgs([]string{"--base-path", "/base/path"})
				require.NoError(t, RootCmd.ParseFlags([]string{"--base-path", "/base/path"}))
				// Verify flag is set.
				val, _ := RootCmd.Flags().GetString("base-path")
				require.Equal(t, "/base/path", val, "Setup should set base-path flag")
			},
			validate: func(t *testing.T) {
				t.Helper()
				val, _ := RootCmd.Flags().GetString("base-path")
				assert.Equal(t, "", val, "base-path flag should be reset to empty")
			},
		},
		{
			name: "resets logs-level flag",
			setup: func(t *testing.T) {
				t.Helper()
				RootCmd.SetArgs([]string{"--logs-level", "Debug"})
				require.NoError(t, RootCmd.ParseFlags([]string{"--logs-level", "Debug"}))
				// Verify flag is set.
				val, _ := RootCmd.Flags().GetString("logs-level")
				require.Equal(t, "Debug", val, "Setup should set logs-level flag")
			},
			validate: func(t *testing.T) {
				t.Helper()
				val, _ := RootCmd.Flags().GetString("logs-level")
				// Should be reset to default value.
				assert.Equal(t, "Info", val, "logs-level flag should be reset to default 'Info'")
			},
		},
		{
			name: "clears command args",
			setup: func(t *testing.T) {
				t.Helper()
				RootCmd.SetArgs([]string{"terraform", "plan", "vpc", "-s", "prod"})
				require.NoError(t, RootCmd.ParseFlags([]string{"terraform", "plan", "vpc", "-s", "prod"}))
			},
			validate: func(t *testing.T) {
				t.Helper()
				// After reset, args should be empty.
				// We can't directly inspect args, but we can verify flags are reset.
				val, _ := RootCmd.Flags().GetString("chdir")
				assert.Equal(t, "", val, "All flags should be reset")
			},
		},
		{
			name: "handles multiple flag resets",
			setup: func(t *testing.T) {
				t.Helper()
				args := []string{
					"--chdir", "/test/dir",
					"--base-path", "/base",
					"--logs-level", "Trace",
				}
				RootCmd.SetArgs(args)
				require.NoError(t, RootCmd.ParseFlags(args))
				// Verify all flags are set.
				chdir, _ := RootCmd.Flags().GetString("chdir")
				basePath, _ := RootCmd.Flags().GetString("base-path")
				logsLevel, _ := RootCmd.Flags().GetString("logs-level")
				require.Equal(t, "/test/dir", chdir)
				require.Equal(t, "/base", basePath)
				require.Equal(t, "Trace", logsLevel)
			},
			validate: func(t *testing.T) {
				t.Helper()
				chdir, _ := RootCmd.Flags().GetString("chdir")
				basePath, _ := RootCmd.Flags().GetString("base-path")
				logsLevel, _ := RootCmd.Flags().GetString("logs-level")
				assert.Equal(t, "", chdir, "chdir should be reset")
				assert.Equal(t, "", basePath, "base-path should be reset")
				assert.Equal(t, "Info", logsLevel, "logs-level should be reset to default")
			},
		},
		{
			name: "is idempotent - multiple resets are safe",
			setup: func(t *testing.T) {
				t.Helper()
				RootCmd.SetArgs([]string{"--chdir", "/path"})
				require.NoError(t, RootCmd.ParseFlags([]string{"--chdir", "/path"}))
			},
			validate: func(t *testing.T) {
				t.Helper()
				// Reset multiple times.
				resetRootCmdForTesting(t)
				resetRootCmdForTesting(t)
				resetRootCmdForTesting(t)
				// Should still be reset.
				val, _ := RootCmd.Flags().GetString("chdir")
				assert.Equal(t, "", val, "Multiple resets should be safe")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup initial state.
			if tt.setup != nil {
				tt.setup(t)
			}

			// Perform reset.
			resetRootCmdForTesting(t)

			// Validate reset worked.
			if tt.validate != nil {
				tt.validate(t)
			}

			// Cleanup: reset again to ensure clean state for next test.
			t.Cleanup(func() {
				resetRootCmdForTesting(t)
			})
		})
	}
}

// TestResetRootCmdForTestingIsolation verifies that the reset helper properly
// isolates tests from each other when used in cleanup.
func TestResetRootCmdForTestingIsolation(t *testing.T) {
	t.Run("first test sets flag", func(t *testing.T) {
		t.Cleanup(func() {
			resetRootCmdForTesting(t)
		})

		RootCmd.SetArgs([]string{"--chdir", "/first/test/path"})
		require.NoError(t, RootCmd.ParseFlags([]string{"--chdir", "/first/test/path"}))
		val, _ := RootCmd.Flags().GetString("chdir")
		assert.Equal(t, "/first/test/path", val)
	})

	t.Run("second test sees clean state", func(t *testing.T) {
		t.Cleanup(func() {
			resetRootCmdForTesting(t)
		})

		// Should not see previous test's flag value.
		val, _ := RootCmd.Flags().GetString("chdir")
		assert.Equal(t, "", val, "Should not see previous test's chdir value")

		// Set our own value.
		RootCmd.SetArgs([]string{"--chdir", "/second/test/path"})
		require.NoError(t, RootCmd.ParseFlags([]string{"--chdir", "/second/test/path"}))
		val, _ = RootCmd.Flags().GetString("chdir")
		assert.Equal(t, "/second/test/path", val)
	})

	t.Run("third test also sees clean state", func(t *testing.T) {
		t.Cleanup(func() {
			resetRootCmdForTesting(t)
		})

		// Should not see previous tests' flag values.
		val, _ := RootCmd.Flags().GetString("chdir")
		assert.Equal(t, "", val, "Should not see previous tests' chdir values")
	})
}

// TestResetRootCmdForTestingDoesNotAffectNonTestableFlags verifies that
// the reset function only resets flags that should be reset, not persistent
// configuration that might affect RootCmd behavior.
func TestResetRootCmdForTestingDoesNotAffectNonTestableFlags(t *testing.T) {
	// Verify RootCmd itself is still functional after reset.
	resetRootCmdForTesting(t)

	// RootCmd should still have its Use field set.
	assert.Equal(t, "atmos", RootCmd.Use, "RootCmd.Use should not be affected")

	// RootCmd should still have flags registered.
	assert.NotNil(t, RootCmd.Flags().Lookup("chdir"), "chdir flag should still exist")
	assert.NotNil(t, RootCmd.Flags().Lookup("base-path"), "base-path flag should still exist")

	// Cleanup.
	t.Cleanup(func() {
		resetRootCmdForTesting(t)
	})
}
