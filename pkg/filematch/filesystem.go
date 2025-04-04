package filematch

import (
	"os"
	"path/filepath"
)

// defaultFileSystem implements FileSystem using the standard library.
type defaultFileSystem struct{}

func newDefaultFileSystem() fileSystem {
	return &defaultFileSystem{}
}

func (fs *defaultFileSystem) Getwd() (string, error) {
	return os.Getwd()
}

func (fs *defaultFileSystem) Walk(root string, walkFn filepath.WalkFunc) error {
	return filepath.Walk(root, walkFn)
}

func (fs *defaultFileSystem) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
