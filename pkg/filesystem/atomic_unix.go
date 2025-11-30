//go:build !windows

package filesystem

import (
	"os"

	"github.com/google/renameio/v2"
)

// WriteFileAtomicUnix uses renameio for atomic file writing on Unix systems.
// This ensures that the file is either fully written or not modified at all,
// preventing partial writes or corruption.
func WriteFileAtomicUnix(filename string, data []byte, perm os.FileMode) error {
	return renameio.WriteFile(filename, data, perm)
}
