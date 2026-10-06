package toolchain

import "golang.org/x/sys/unix"

// toolVersionsPreserveCase reads the filesystem's lookup behavior without
// creating a file. Darwin declares _PC_CASE_SENSITIVE as 11 in sys/unistd.h.
func toolVersionsPreserveCase(path string) (bool, error) {
	const pathconfCaseSensitive = 11
	sensitive, err := unix.Pathconf(path, pathconfCaseSensitive)
	return sensitive != 0, err
}
