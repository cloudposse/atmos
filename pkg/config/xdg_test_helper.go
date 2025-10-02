package config

import (
	"os"
	"sync"
	"testing"

	"github.com/adrg/xdg"
)

// xdgMutex serializes access to XDG operations in tests to prevent
// concurrent modifications of the global XDG state.
var xdgMutex sync.Mutex

// withTestXDGHome sets up a test-specific XDG_CACHE_HOME and ensures
// proper cleanup and serialization of XDG operations.
func withTestXDGHome(t *testing.T, testDir string) func() {
	// Lock the mutex to prevent concurrent XDG modifications.
	xdgMutex.Lock()

	// Save the original XDG_CACHE_HOME.
	//nolint:forbidigo // Test helper needs to save/restore environment
	originalXDG := os.Getenv("XDG_CACHE_HOME")

	// Use t to register the cleanup on test failure
	t.Helper()

	// Set the test directory as XDG_CACHE_HOME.
	os.Setenv("XDG_CACHE_HOME", testDir)

	// Reload XDG to pick up the environment change.
	xdg.Reload()

	// Return cleanup function.
	return func() {
		// Restore the original XDG_CACHE_HOME.
		os.Setenv("XDG_CACHE_HOME", originalXDG)
		// Reload XDG to restore the original state.
		xdg.Reload()
		// Unlock the mutex to allow other tests to proceed.
		xdgMutex.Unlock()
	}
}
