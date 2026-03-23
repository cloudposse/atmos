//go:build windows

package filesystem

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWriteFileAtomicWindows_Create verifies that WriteFileAtomicWindows creates a new file.
func TestWriteFileAtomicWindows_Create(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "new-file.txt")
	content := []byte("hello windows atomic create")

	err := WriteFileAtomicWindows(filePath, content, 0o644)
	require.NoError(t, err, "WriteFileAtomicWindows should create a new file")

	got, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

// TestWriteFileAtomicWindows_Overwrite verifies that WriteFileAtomicWindows overwrites an
// existing file atomically (remove-before-rename path on Windows).
func TestWriteFileAtomicWindows_Overwrite(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "existing-file.txt")

	// Write initial content.
	require.NoError(t, os.WriteFile(filePath, []byte("initial content"), 0o644))

	newContent := []byte("overwritten content via WriteFileAtomicWindows")
	err := WriteFileAtomicWindows(filePath, newContent, 0o644)
	require.NoError(t, err, "WriteFileAtomicWindows should overwrite an existing file")

	got, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, newContent, got, "file must contain new content after overwrite")
}

// TestWriteFileAtomicWindows_RemoveBeforeRename exercises the remove-before-rename code path
// by simulating the scenario where the destination file already exists.
// On Windows, os.Rename fails if the target exists so WriteFileAtomicWindows removes it first.
func TestWriteFileAtomicWindows_RemoveBeforeRename(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "replace-me.txt")

	// Create a non-empty existing file.
	require.NoError(t, os.WriteFile(filePath, []byte("old data"), 0o644))

	// Overwrite multiple times to ensure the remove-before-rename path is exercised reliably.
	for i := range 3 {
		content := []byte("iteration " + string(rune('0'+i)))
		err := WriteFileAtomicWindows(filePath, content, 0o644)
		require.NoError(t, err)

		got, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, content, got)
	}
}
