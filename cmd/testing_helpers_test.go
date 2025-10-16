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
	flags := []string{
		"chdir",
		"base-path",
		"config-dir",
		"stacks-dir",
		"components-dir",
		"workflows-dir",
		"logs-file",
		"logs-level",
	}

	for _, flag := range flags {
		// Check if flag exists before trying to set it (some flags may be optional).
		if f := RootCmd.Flags().Lookup(flag); f != nil {
			_ = RootCmd.Flags().Set(flag, f.DefValue)
		}
	}
}
