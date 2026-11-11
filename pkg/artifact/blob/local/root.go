package local

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// openRoot confines object operations to the configured directory, including through symlinks.
func (s *Store) openRoot(create bool) (*os.Root, error) {
	if create {
		if err := os.MkdirAll(s.root, s.dirPerm); err != nil {
			return nil, err
		}
	}
	return os.OpenRoot(s.root)
}

func rootOpenFile(root *os.Root, name string) (*os.File, error) {
	attempts := 1
	if runtime.GOOS == "windows" {
		attempts = openAttempts
	}
	return openFileWith(name, attempts, func(name string) (*os.File, error) {
		return root.OpenFile(name, os.O_RDONLY|nonblockFlag, 0)
	})
}

func rootReadFile(root *os.Root, name string) ([]byte, error) {
	file, err := rootOpenFile(root, name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}

func rootCreateTemp(root *os.Root, dir string, dirPerm, filePerm os.FileMode) (*os.File, string, error) {
	var lastErr error
	for range renameAttempts {
		if err := root.MkdirAll(dir, dirPerm); err != nil {
			return nil, "", err
		}
		name, err := tempName()
		if err != nil {
			return nil, "", err
		}
		name = filepath.Join(dir, name)
		file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, filePerm)
		if err == nil {
			return file, name, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, "", err
		}
		lastErr = err
	}
	return nil, "", lastErr
}

func rootRenameReplace(root *os.Root, src, dst string) error {
	attempts := 1
	if runtime.GOOS == "windows" {
		attempts = renameAttempts
	}
	var err error
	for i := range attempts {
		if err = root.Rename(src, dst); err == nil {
			return nil
		}
		if i < attempts-1 {
			time.Sleep(renameBackoff)
		}
	}
	return err
}

func rootRemoveEmptyDirs(root *os.Root, dir string) {
	for dir != "." && dir != string(filepath.Separator) {
		file, err := root.Open(dir)
		if err != nil {
			return
		}
		_, readErr := file.ReadDir(1)
		_ = file.Close()
		if !errors.Is(readErr, io.EOF) || root.Remove(dir) != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}
