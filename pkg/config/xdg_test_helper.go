package config

import (
	"testing"

	"github.com/adrg/xdg"
)

// withTestXDGHome sets up a test-specific XDG_CACHE_HOME and ensures
// proper cleanup via t.Setenv() automatic restoration.
func withTestXDGHome(t *testing.T, testDir string) func() {
	t.Helper()

	// Set the test directory as XDG_CACHE_HOME.
	// t.Setenv automatically saves the original value and restores it when the test completes.
	t.Setenv("XDG_CACHE_HOME", testDir)

	// Reload XDG to pick up the environment change.
	xdg.Reload()

	// Return cleanup function that reloads XDG to reset state.
	return func() {
		// Reload XDG to restore the original state.
		// The environment variable is automatically restored by t.Setenv.
		xdg.Reload()
	}
}
