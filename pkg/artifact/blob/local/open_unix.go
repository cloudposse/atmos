//go:build unix

package local

import (
	"os"

	"golang.org/x/sys/unix"
)

const nonblockFlag = unix.O_NONBLOCK

func openRead(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|unix.O_NONBLOCK, 0)
}
