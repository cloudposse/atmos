package config

import (
	"os"
	"sync"
	"testing"

	"github.com/adrg/xdg"
)

// xdgMutex serializes access to XDG operations in tests to prevent
// concurrent modifications of the global XDG state.
// This is necessary because XDG_CACHE_HOME affects process-wide state
// and tests running in parallel would interfere with each other.
var xdgMutex sync.Mutex

// withTestXDGHome sets up a test-specific XDG_CACHE_HOME with proper
// serialization and cleanup.
//
// This function uses a mutex to ensure only one test modifies XDG state
// at a time, preventing race conditions when tests run in parallel.
func withTestXDGHome(t *testing.T, testDir string) func() {
	t.Helper()

	// Lock the mutex to serialize XDG modifications across all tests.
	// This prevents parallel tests from corrupting each other's XDG state.
	xdgMutex.Lock()

	// Save the original value before modification.
	//nolint:forbidigo // Need to save original value before t.Setenv modifies it
	originalXDG := os.Getenv("XDG_CACHE_HOME")

	// Set the test directory as XDG_CACHE_HOME.
	// We use os.Setenv instead of t.Setenv because t.Setenv cannot be used
	// when tests (or their children) might run in parallel, which is common
	// in the full test suite.
	os.Setenv("XDG_CACHE_HOME", testDir)

	// Reload XDG to pick up the environment change.
	xdg.Reload()

	// Return cleanup function.
	return func() {
		// Restore the original XDG_CACHE_HOME value.
		os.Setenv("XDG_CACHE_HOME", originalXDG)

		// Reload XDG to pick up the restored value.
		xdg.Reload()

		// Release the mutex to allow other tests to proceed.
		xdgMutex.Unlock()
	}
}

// withTestXDGConfigHome sets up a test-specific XDG_CONFIG_HOME with proper
// serialization and cleanup.
//
// This function uses a mutex to ensure only one test modifies XDG state
// at a time, preventing race conditions when tests run in parallel.
//
// This is specifically for profile-related tests that need to isolate
// XDG_CONFIG_HOME from the system's actual config directory.
func withTestXDGConfigHome(t *testing.T, testDir string) func() {
	t.Helper()

	// Lock the mutex to serialize XDG modifications across all tests.
	// This prevents parallel tests from corrupting each other's XDG state.
	xdgMutex.Lock()

	// Save the original values before modification.
	//nolint:forbidigo // Need to save original value before t.Setenv modifies it
	originalXDGConfig := os.Getenv("XDG_CONFIG_HOME")
	//nolint:forbidigo // Need to save original value before t.Setenv modifies it
	originalAtmosXDGConfig := os.Getenv("ATMOS_XDG_CONFIG_HOME")

	// Set the test directory as XDG_CONFIG_HOME and ATMOS_XDG_CONFIG_HOME.
	// We use os.Setenv instead of t.Setenv because t.Setenv cannot be used
	// when tests (or their children) might run in parallel, which is common
	// in the full test suite.
	os.Setenv("XDG_CONFIG_HOME", testDir)
	os.Setenv("ATMOS_XDG_CONFIG_HOME", testDir)

	// Reload XDG to pick up the environment changes.
	xdg.Reload()

	// Return cleanup function.
	return func() {
		// Restore the original XDG_CONFIG_HOME and ATMOS_XDG_CONFIG_HOME values.
		os.Setenv("XDG_CONFIG_HOME", originalXDGConfig)
		os.Setenv("ATMOS_XDG_CONFIG_HOME", originalAtmosXDGConfig)

		// Reload XDG to pick up the restored values.
		xdg.Reload()

		// Release the mutex to allow other tests to proceed.
		xdgMutex.Unlock()
	}
}
