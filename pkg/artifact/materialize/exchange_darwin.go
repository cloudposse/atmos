//go:build darwin

package materialize

import (
	"os"
	"runtime"

	"golang.org/x/sys/unix"
)

func exchangeDirectories(stageParent *os.File, stage string, destParent *os.File, dest string) error {
	err := unix.RenameatxNp(int(stageParent.Fd()), stage, int(destParent.Fd()), dest, unix.RENAME_SWAP)
	runtime.KeepAlive(stageParent)
	runtime.KeepAlive(destParent)
	return err
}

func renameExclusive(from, to string) error {
	return unix.RenameatxNp(unix.AT_FDCWD, from, unix.AT_FDCWD, to, unix.RENAME_EXCL)
}
