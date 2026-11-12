//go:build !unix

package local

import "os"

const nonblockFlag = 0

func openRead(path string) (*os.File, error) { return os.Open(path) }
