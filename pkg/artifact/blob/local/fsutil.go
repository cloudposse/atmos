package local

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const (
	renameAttempts = 8
	renameBackoff  = 5 * time.Millisecond

	// The openAttempts constant bounds the Windows retry for reads that race a concurrent rename-over.
	openAttempts = 20
)

func mkdirAll(dir string, perm os.FileMode) error {
	return os.MkdirAll(dir, perm)
}

// createTemp creates an exclusive temporary file in dir, creating dir when needed. A concurrent
// delete may remove an empty directory between creation and use, so it retries a few times.
func createTemp(dir string, dirPerm, filePerm os.FileMode) (*os.File, error) {
	var lastErr error
	for range renameAttempts {
		if err := mkdirAll(dir, dirPerm); err != nil {
			return nil, err
		}
		name, err := tempName()
		if err != nil {
			return nil, err
		}
		file, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, filePerm)
		if err == nil {
			return file, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		lastErr = err
	}
	return nil, lastErr
}

func tempName() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return tempPrefix + hex.EncodeToString(raw[:]), nil
}

// renameReplace atomically moves src over dst. Windows rejects a rename while another handle is
// open on dst, so it retries briefly there.
func renameReplace(src, dst string) error {
	attempts := 1
	if runtime.GOOS == "windows" {
		attempts = renameAttempts
	}
	var err error
	for i := range attempts {
		if err = os.Rename(src, dst); err == nil {
			return nil
		}
		if i < attempts-1 {
			time.Sleep(renameBackoff)
		}
	}
	return err
}

// openFile opens path for reading. Windows refuses to open a file with a sharing violation while a
// concurrent writer is renaming over it, so it retries briefly there. A missing file is reported
// immediately because absence is a normal answer.
func openFile(path string) (*os.File, error) {
	attempts := 1
	if runtime.GOOS == "windows" {
		attempts = openAttempts
	}
	var (
		file *os.File
		err  error
	)
	for i := range attempts {
		file, err = openRead(path)
		if err == nil || errors.Is(err, os.ErrNotExist) {
			return file, err
		}
		if i < attempts-1 {
			time.Sleep(renameBackoff)
		}
	}
	return nil, err
}

// readFile returns the contents of path, retrying a transient Windows sharing violation like openFile.
func readFile(path string) ([]byte, error) {
	file, err := openFile(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}

// removeEmptyDirs removes empty directories from dir up to, but never including, root.
func removeEmptyDirs(root, dir string) {
	for dir != root && dir != "." && dir != string(filepath.Separator) && len(dir) > len(root) {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		if os.Remove(dir) != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}
