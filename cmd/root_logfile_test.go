package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLogFileStaysOpen verifies that the log file stays open during program execution.
func TestLogFileStaysOpen(t *testing.T) {
	// Create a temporary log file.
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")

	// Set up the log file configuration.
	t.Setenv("ATMOS_LOGS_FILE", logFile)
	t.Setenv("ATMOS_LOGS_LEVEL", "Trace")

	// Simulate opening the log file (this would happen in setupLogger).
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	require.NoError(t, err)

	// Store it in our package variable.
	logFileHandle = file

	// Write some data to verify the file is open.
	_, err = file.WriteString("Test log entry\n")
	assert.NoError(t, err)

	// Verify the file is still open.
	stat1, err := file.Stat()
	assert.NoError(t, err)
	assert.NotNil(t, stat1)

	// Sleep briefly to simulate program execution.
	time.Sleep(10 * time.Millisecond)

	// Write more data to verify it's still open.
	_, err = file.WriteString("Another test log entry\n")
	assert.NoError(t, err)

	// Clean up using our cleanup function.
	cleanupLogFile()

	// Verify the file was closed by trying to write to it.
	_, err = file.WriteString("This should fail\n")
	assert.Error(t, err, "Writing to closed file should fail")

	// Verify the log file has the expected content.
	content, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Test log entry")
	assert.Contains(t, string(content), "Another test log entry")
}

// TestCleanupFunction verifies the Cleanup function works correctly.
func TestCleanupFunction(t *testing.T) {
	// Create a temporary log file.
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "cleanup_test.log")

	// Open a file and store it.
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	require.NoError(t, err)
	logFileHandle = file

	// Write some data.
	_, err = file.WriteString("Before cleanup\n")
	assert.NoError(t, err)

	// Call the public Cleanup function.
	Cleanup()

	// Verify the handle was cleared.
	assert.Nil(t, logFileHandle)

	// Verify the file was closed.
	_, err = file.WriteString("After cleanup - should fail\n")
	assert.Error(t, err, "Writing to closed file should fail")

	// Calling Cleanup again should be safe (no panic).
	assert.NotPanics(t, func() {
		Cleanup()
	})
}
