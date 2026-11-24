package filesystem

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/cloudposse/atmos/pkg/perf"
)

// maxSymlinkHops bounds how many chained symlinks ResolveSymlinkTarget follows, like the OS
// loop protection (a cycle never resolves).
const maxSymlinkHops = 40

// WriteFileAtomicOrInPlace replaces name with data atomically (temporary file and rename) and,
// when the kernel refuses that replace, writes the same bytes into the existing file instead.
//
// It is for files Atmos shares with other tools and does not own the location of, such as
// Docker's config.json or the Azure CLI token cache. Such a file can be mounted as a single file
// (a devcontainer bind mount or a Kubernetes subPath), where a rename onto it fails with EBUSY,
// or live in a directory the user cannot write to although the file itself is writable, where
// the temporary file cannot be created. Older Atmos versions wrote these files in place, so they
// worked there. The fallback applies only when the replace failed with EBUSY, EXDEV, EROFS or a
// permission error and name is an existing regular file. An in-place write keeps the file's
// permission bits, except that, where Unix permission bits apply, it first removes any bit perm
// does not grant, so a token cache that other users can read is never rewritten with new secrets:
// the atomic write would have created a new file with perm. When the mode cannot be restricted,
// the in-place write is refused.
// When the in-place write fails, both errors are returned.
func WriteFileAtomicOrInPlace(fsys FileSystem, name string, data []byte, perm os.FileMode) error {
	defer perf.Track(nil, "filesystem.WriteFileAtomicOrInPlace")()

	atomicErr := fsys.WriteFileAtomic(name, data, perm)
	if atomicErr == nil || !isReplaceRefused(atomicErr) {
		return atomicErr
	}
	info, err := fsys.Stat(name)
	if err != nil || !info.Mode().IsRegular() {
		return atomicErr
	}
	if current := info.Mode().Perm(); permissionBitsEnforced && current&^perm != 0 {
		if err := fsys.Chmod(name, current&perm); err != nil {
			return errors.Join(atomicErr, err)
		}
	}
	if err := fsys.WriteFile(name, data, perm); err != nil {
		return errors.Join(atomicErr, err)
	}
	return nil
}

// permissionBitsEnforced is false on Windows, where a file reports mode 0666 (or 0444) and Chmod only
// toggles the read-only attribute, so restricting Unix permission bits before an in-place write is
// meaningless there.
var permissionBitsEnforced = runtime.GOOS != "windows"

// isReplaceRefused reports whether err is a failure an in-place write can work around: the
// rename target is busy (a mount point), on another device, or the directory cannot be written.
func isReplaceRefused(err error) bool {
	return errors.Is(err, syscall.EBUSY) ||
		errors.Is(err, syscall.EXDEV) ||
		errors.Is(err, syscall.EROFS) ||
		errors.Is(err, fs.ErrPermission)
}

// ResolveSymlinkTarget returns the file a write to path must replace so that a symlinked path
// stays a symlink. A fully resolvable link yields its final target. A dangling link (its target
// does not exist yet) is followed link by link, so the write creates the target instead of
// replacing the link with a regular file. A path that is not a symlink, or a cycle, is returned
// unchanged.
func ResolveSymlinkTarget(path string) string {
	defer perf.Track(nil, "filesystem.ResolveSymlinkTarget")()

	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}

	current := path
	for range maxSymlinkHops {
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			return current
		}
		dest, err := os.Readlink(current)
		if err != nil {
			return current
		}
		if !filepath.IsAbs(dest) {
			dest = filepath.Join(filepath.Dir(current), dest)
		}
		current = dest
	}
	return path
}
