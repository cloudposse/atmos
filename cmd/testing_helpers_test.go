package cmd

import "testing"

// resetRootCmdForTesting resets the global RootCmd state to prevent test pollution.
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
//
// Or inline when needed:
//
//	resetRootCmdForTesting(t)
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
