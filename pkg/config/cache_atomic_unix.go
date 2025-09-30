//go:build !windows

package config

import (
	"os"

	"github.com/google/renameio/v2"
)

func init() {
	// Set the platform-specific atomic write function for Unix systems.
	writeFileAtomic = writeFileAtomicUnix
}

// writeFileAtomicUnix uses renameio for atomic file writing on Unix systems.
func writeFileAtomicUnix(filename string, data []byte, perm os.FileMode) error {
	return renameio.WriteFile(filename, data, perm)
}
