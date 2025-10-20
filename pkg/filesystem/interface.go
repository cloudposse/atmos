package filesystem

import (
	"io"
	"os"
	"path/filepath"
)

// FileSystem defines filesystem operations for testability.
// This interface allows mocking of file I/O operations in tests.
//
//go:generate go run go.uber.org/mock/mockgen@latest -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
type FileSystem interface {
	// Open opens a file for reading.
	Open(name string) (*os.File, error)

	// Create creates or truncates a file.
	Create(name string) (*os.File, error)

	// Stat returns file info.
	Stat(name string) (os.FileInfo, error)

	// MkdirAll creates a directory and all parent directories.
	MkdirAll(path string, perm os.FileMode) error

	// Chmod changes file permissions.
	Chmod(name string, mode os.FileMode) error

	// MkdirTemp creates a temporary directory.
	MkdirTemp(dir, pattern string) (string, error)

	// CreateTemp creates a temporary file in the directory dir with a name beginning with pattern.
	// If dir is the empty string, CreateTemp uses the default directory for temporary files.
	// It returns the opened file and its name.
	CreateTemp(dir, pattern string) (*os.File, error)

	// WriteFile writes data to a file.
	WriteFile(name string, data []byte, perm os.FileMode) error

	// ReadFile reads a file.
	ReadFile(name string) ([]byte, error)

	// Remove removes a file or empty directory.
	Remove(name string) error

	// RemoveAll removes a path and any children.
	RemoveAll(path string) error

	// Walk walks the file tree.
	Walk(root string, fn filepath.WalkFunc) error

	// Getwd returns the current working directory.
	Getwd() (string, error)
}

// GlobMatcher defines glob pattern matching operations.
//
//go:generate go run go.uber.org/mock/mockgen@latest -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
type GlobMatcher interface {
	// GetGlobMatches returns all files matching the glob pattern.
	GetGlobMatches(pattern string) ([]string, error)

	// PathMatch returns true if name matches the pattern.
	PathMatch(pattern, name string) (bool, error)
}

// IOCopier defines I/O copy operations.
//
//go:generate go run go.uber.org/mock/mockgen@latest -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
type IOCopier interface {
	// Copy copies from src to dst until EOF or error.
	Copy(dst io.Writer, src io.Reader) (written int64, err error)
}
