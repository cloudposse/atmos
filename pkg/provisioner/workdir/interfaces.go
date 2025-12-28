package workdir

import (
	"io/fs"
)

//go:generate go run go.uber.org/mock/mockgen@latest -source=interfaces.go -destination=mock_interfaces_test.go -package=workdir

// FileSystem abstracts file system operations for testability.
type FileSystem interface {
	// MkdirAll creates a directory along with any necessary parents.
	MkdirAll(path string, perm fs.FileMode) error

	// RemoveAll removes path and any children it contains.
	RemoveAll(path string) error

	// Exists checks if a path exists.
	Exists(path string) bool

	// ReadFile reads the contents of a file.
	ReadFile(path string) ([]byte, error)

	// WriteFile writes data to a file with the given permissions.
	WriteFile(path string, data []byte, perm fs.FileMode) error

	// CopyDir recursively copies a directory from src to dst.
	CopyDir(src, dst string) error

	// Walk walks the file tree rooted at root, calling fn for each file or directory.
	Walk(root string, fn fs.WalkDirFunc) error

	// Stat returns file info for the given path.
	Stat(path string) (fs.FileInfo, error)
}

// Hasher computes content hashes for change detection.
type Hasher interface {
	// HashDir computes a hash of all files in a directory.
	HashDir(path string) (string, error)

	// HashFile computes a hash of a single file.
	HashFile(path string) (string, error)
}

// PathFilter filters paths based on include/exclude patterns.
type PathFilter interface {
	// Match returns true if the path should be included.
	Match(path string, includedPaths, excludedPaths []string) (bool, error)
}
