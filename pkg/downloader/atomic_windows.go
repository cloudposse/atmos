//go:build windows

package downloader

import (
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
)

// writeFileAtomicDefault provides atomic-like file writing on Windows.
func writeFileAtomicDefault(filename string, data []byte, perm os.FileMode) error {
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

	// Apply the requested permissions to the temporary file.
	if err := os.Chmod(tmpName, perm); err != nil {
		os.Remove(tmpName) // Clean up temp file.
		return fmt.Errorf("%w: failed to chmod temp file %s: %v", errUtils.ErrFileOperation, tmpName, err)
	}

	// On Windows, we need to remove the target file first if it exists.
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
