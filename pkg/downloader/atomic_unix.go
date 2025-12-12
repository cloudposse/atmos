//go:build !windows

package downloader

import (
	"os"

	"github.com/google/renameio/v2"
)

// writeFileAtomicDefault uses renameio for atomic file writing on Unix systems.
func writeFileAtomicDefault(filename string, data []byte, perm os.FileMode) error {
	return renameio.WriteFile(filename, data, perm)
}
