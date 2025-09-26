//go:build windows

package config

import (
	"os"
	"path/filepath"
)

func init() {
	// Set the platform-specific atomic write function for Windows.
	writeFileAtomic = writeFileAtomicWindows
}

// writeFileAtomicWindows provides atomic-like file writing on Windows.
// Since renameio doesn't work well on Windows due to file locking issues,
// we use a simple write with a temporary file and rename approach.
func writeFileAtomicWindows(filename string, data []byte, perm os.FileMode) error {
	// Create a temporary file in the same directory.
	dir := filepath.Dir(filename)
	tmpFile, err := os.CreateTemp(dir, ".tmp-")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()

	// Clean up temporary file on error.
	defer func() {
		if tmpFile != nil {
			tmpFile.Close()
			os.Remove(tmpName)
		}
	}()

	// Write data to temporary file.
	if _, err := tmpFile.Write(data); err != nil {
		return err
	}

	// Close the temporary file before renaming.
	if err := tmpFile.Close(); err != nil {
		return err
	}
	tmpFile = nil // Mark as closed for defer cleanup.

	// On Windows, we need to remove the target file first if it exists.
	// This is because Windows doesn't allow renaming over an existing file.
	if _, err := os.Stat(filename); err == nil {
		if err := os.Remove(filename); err != nil {
			os.Remove(tmpName) // Clean up temp file.
			return err
		}
	}

	// Rename temporary file to target.
	if err := os.Rename(tmpName, filename); err != nil {
		os.Remove(tmpName) // Clean up temp file.
		return err
	}

	return nil
}
