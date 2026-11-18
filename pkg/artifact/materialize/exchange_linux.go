//go:build linux

package materialize

import (
	"os"
	"runtime"

	"golang.org/x/sys/unix"
)

func exchangeDirectories(stageParent *os.File, stage string, destParent *os.File, dest string) error {
	err := unix.Renameat2(int(stageParent.Fd()), stage, int(destParent.Fd()), dest, unix.RENAME_EXCHANGE)
	runtime.KeepAlive(stageParent)
	runtime.KeepAlive(destParent)
	return err
}

func renameExclusive(from, to string) error {
	return unix.Renameat2(unix.AT_FDCWD, from, unix.AT_FDCWD, to, unix.RENAME_NOREPLACE)
}
