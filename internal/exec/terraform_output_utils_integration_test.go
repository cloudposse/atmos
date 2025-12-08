package exec

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryOnWindows_FileOperations(t *testing.T) {
	// Create a temporary directory for testing.
	tmpDir := t.TempDir()

	testFile := filepath.Join(tmpDir, "test.txt")

	// Create a test file.
	err := os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	// Test file deletion with retry logic.
	err = retryOnWindows(func() error {
		return os.Remove(testFile)
	})
	assert.NoError(t, err)

	// Verify file was deleted.
	_, err = os.Stat(testFile)
	assert.True(t, os.IsNotExist(err), "File should be deleted")
}

func TestRetryOnWindows_SimulatedLockingScenario(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skipf("This test simulates Windows-specific file locking behavior")
	}

	tmpDir := t.TempDir()

	testFile := filepath.Join(tmpDir, "state.tfstate")

	// Create a test file.
	err := os.WriteFile(testFile, []byte("terraform state"), 0o644)
	require.NoError(t, err)

	// Open the file to simulate it being locked.
	file, err := os.Open(testFile)
	require.NoError(t, err)

	// Try to delete while file is open (this typically fails on Windows).
	deleteErr := os.Remove(testFile)

	if deleteErr != nil {
		// File is locked, close it and try with retry logic.
		file.Close()

		// Small delay to ensure handle is released.
		time.Sleep(50 * time.Millisecond)

		// Now deletion with retry should work.
		err = retryOnWindows(func() error {
			return os.Remove(testFile)
		})
		assert.NoError(t, err, "Retry logic should handle file deletion after lock is released")
	} else {
		// If deletion succeeded immediately, just close the file.
		file.Close()
	}

	// Verify file was deleted.
	_, err = os.Stat(testFile)
	assert.True(t, os.IsNotExist(err), "File should be deleted")
}

func TestWindowsFileDelay_Timing(t *testing.T) {
	// Test that the delay function behaves correctly on the current platform.
	start := time.Now()
	windowsFileDelay()
	elapsed := time.Since(start)

	if runtime.GOOS == "windows" {
		// On Windows, expect a delay.
		assert.GreaterOrEqual(t, elapsed.Milliseconds(), int64(90), "Expected at least 90ms delay on Windows")
	} else {
		// On other platforms, expect no significant delay.
		assert.Less(t, elapsed.Milliseconds(), int64(10), "Expected no significant delay on non-Windows")
	}
}
