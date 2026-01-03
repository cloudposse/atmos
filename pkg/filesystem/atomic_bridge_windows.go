//go:build windows

package filesystem

import "os"

// writeFileAtomicImpl is the Windows implementation bridge.
func writeFileAtomicImpl(filename string, data []byte, perm os.FileMode) error {
	return WriteFileAtomicWindows(filename, data, perm)
}
