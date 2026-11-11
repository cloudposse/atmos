package local

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
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

func tempName() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return tempPrefix + hex.EncodeToString(raw[:]), nil
}

// openFile opens path for reading. Windows refuses to open a file with a sharing violation while a
// concurrent writer is renaming over it, so it retries briefly there. A missing file is reported
// immediately because absence is a normal answer.
func openFile(path string) (*os.File, error) {
	attempts := 1
	if runtime.GOOS == "windows" {
		attempts = openAttempts
	}
	return openFileWith(path, attempts, openRead)
}

func openFileWith(path string, attempts int, open func(string) (*os.File, error)) (*os.File, error) {
	var (
		file *os.File
		err  error
	)
	for i := range attempts {
		file, err = open(path)
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
