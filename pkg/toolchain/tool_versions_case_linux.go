package toolchain

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// toolVersionsPreserveCase honors Linux casefold directories when supported by
// the filesystem. Unsupported inode flags do not establish case sensitivity;
// unknown filesystems conservatively share locks across case variants.
func toolVersionsPreserveCase(path string) (bool, error) {
	dir, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer dir.Close()
	flags, err := unix.IoctlGetInt(int(dir.Fd()), unix.FS_IOC_GETFLAGS)
	if errors.Is(err, unix.ENOTTY) || errors.Is(err, unix.EOPNOTSUPP) {
		return toolVersionsLinuxFallbackCasePolicy(path, int(dir.Fd()))
	}
	const casefoldDirectory = 0x40000000
	return flags&casefoldDirectory == 0, err
}

// toolVersionsLinuxFallbackCasePolicy recognizes native virtual filesystems
// whose lookup rules are fixed by Linux. Everything else uses the conservative
// policy, including exFAT and remote filesystems with configurable case rules.
func toolVersionsLinuxFallbackCasePolicy(path string, fd int) (bool, error) {
	var filesystem unix.Statfs_t
	if err := unix.Fstatfs(fd, &filesystem); err != nil {
		return false, err
	}
	switch filesystem.Type {
	case unix.PROC_SUPER_MAGIC, unix.SYSFS_MAGIC, unix.TMPFS_MAGIC:
		return true, nil
	default:
		return toolVersionsFallbackCasePolicy(path)
	}
}
