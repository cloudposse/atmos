//go:build !windows

package filesystem

import (
	"os"

	"github.com/google/renameio/v2"
)

// WriteFileAtomicUnix uses renameio for atomic file writing on Unix systems.
// This ensures atomic visibility (readers never see truncated files) via
// temp file + rename. Note: durability depends on fsync behavior.
func WriteFileAtomicUnix(filename string, data []byte, perm os.FileMode) error {
	return renameio.WriteFile(filename, data, perm)
}
