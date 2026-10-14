package toolchain

import "golang.org/x/sys/unix"

// toolVersionsPreserveCase recognizes FreeBSD filesystems with fixed native
// case-sensitive lookup. Filesystems such as msdosfs and configurable ZFS use
// stable conservative lock keys rather than guessing from directory entries.
func toolVersionsPreserveCase(path string) (bool, error) {
	var filesystem unix.Statfs_t
	if err := unix.Statfs(path, &filesystem); err != nil {
		return false, err
	}
	switch unix.ByteSliceToString(filesystem.Fstypename[:]) {
	case "ufs", "tmpfs", "procfs", "devfs":
		return true, nil
	default:
		return toolVersionsFallbackCasePolicy(path)
	}
}
