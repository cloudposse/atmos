//go:build !windows

package filesystem

import "os"

// writeFileAtomicImpl is the Unix implementation bridge.
func writeFileAtomicImpl(filename string, data []byte, perm os.FileMode) error {
	return WriteFileAtomicUnix(filename, data, perm)
}
