//go:build !windows
// +build !windows

package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupRawMode(t *testing.T) {
	// This test verifies the function returns a cleanup function
	// Note: We can't easily test terminal operations in unit tests

	cleanup := setupRawMode()
	assert.NotNil(t, cleanup, "Should return a cleanup function")

	// Cleanup function should be safe to call
	assert.NotPanics(t, func() {
		cleanup()
	})
}

func TestSetupPTYResize(t *testing.T) {
	// Create a temporary file to simulate a PTY
	tmpFile, err := os.CreateTemp("", "pty-test-*")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Setup PTY resize should return a cleanup function
	cleanup := setupPTYResize(tmpFile)
	assert.NotNil(t, cleanup, "Should return a cleanup function")

	// Cleanup function should be safe to call
	assert.NotPanics(t, func() {
		cleanup()
	})
}

func TestRunWithPTY(t *testing.T) {
	// This is a complex function that requires a real command
	// We'll test with a simple echo command that should work on all platforms

	t.Run("command not found", func(t *testing.T) {
		// Note: We can't easily create an exec.Cmd with a custom Path
		// This test would require more complex setup or mocking
		t.Skip("Requires complex PTY setup that's hard to test in unit tests")
	})

	t.Run("cleanup functions are called", func(t *testing.T) {
		// This test would verify that all deferred cleanup functions are called
		// but requires PTY support which may not be available in all test environments
		t.Skip("Requires PTY support which may not be available in CI")
	})
}
