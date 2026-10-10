//go:build !unix

package toolchain

// isReadOnlyFilesystemError reports whether err is a read-only filesystem error.
// Platforms without EROFS report read-only media as permission errors, which
// callers already handle through fs.ErrPermission.
func isReadOnlyFilesystemError(_ error) bool {
	return false
}
