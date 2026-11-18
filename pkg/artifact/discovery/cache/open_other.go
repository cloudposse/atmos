//go:build !unix

package cache

import "os"

func openSnapshot(path string) (*os.File, error) { return os.Open(path) }
