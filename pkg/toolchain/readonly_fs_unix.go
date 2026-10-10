//go:build unix

package toolchain

import (
	"errors"
	"syscall"
)

// isReadOnlyFilesystemError reports whether err is a read-only filesystem error (EROFS).
func isReadOnlyFilesystemError(err error) bool {
	return errors.Is(err, syscall.EROFS)
}
