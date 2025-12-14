package filesystem

import (
	"io"
	"os"
	"path/filepath"
)

// OSFileSystem is the default implementation that uses the os package.
type OSFileSystem struct{}

// NewOSFileSystem creates a new OS filesystem implementation.
func NewOSFileSystem() *OSFileSystem {
	return &OSFileSystem{}
}

// Open opens a file for reading.
func (o *OSFileSystem) Open(name string) (*os.File, error) {
	return os.Open(name)
}

// Create creates or truncates a file.
func (o *OSFileSystem) Create(name string) (*os.File, error) {
	return os.Create(name)
}

// Stat returns file info.
func (o *OSFileSystem) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

// MkdirAll creates a directory and all parent directories.
func (o *OSFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// Chmod changes file permissions.
func (o *OSFileSystem) Chmod(name string, mode os.FileMode) error {
	return os.Chmod(name, mode)
}

// MkdirTemp creates a temporary directory.
func (o *OSFileSystem) MkdirTemp(dir, pattern string) (string, error) {
	return os.MkdirTemp(dir, pattern)
}

// CreateTemp creates a temporary file in the directory dir with a name beginning with pattern.
func (o *OSFileSystem) CreateTemp(dir, pattern string) (*os.File, error) {
	return os.CreateTemp(dir, pattern)
}

// WriteFile writes data to a file.
func (o *OSFileSystem) WriteFile(name string, data []byte, perm os.FileMode) error {
	return os.WriteFile(name, data, perm)
}

// ReadFile reads a file.
func (o *OSFileSystem) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

// Remove removes a file or empty directory.
func (o *OSFileSystem) Remove(name string) error {
	return os.Remove(name)
}

// RemoveAll removes a path and any children.
func (o *OSFileSystem) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

// Walk walks the file tree.
func (o *OSFileSystem) Walk(root string, fn filepath.WalkFunc) error {
	return filepath.Walk(root, fn)
}

// Getwd returns the current working directory.
func (o *OSFileSystem) Getwd() (string, error) {
	return os.Getwd()
}

// WriteFileAtomic writes data to a file atomically using platform-specific implementations.
func (o *OSFileSystem) WriteFileAtomic(name string, data []byte, perm os.FileMode) error {
	return writeFileAtomicImpl(name, data, perm)
}

// OSGlobMatcher is the default implementation that uses pkg/utils.
type OSGlobMatcher struct{}

// NewOSGlobMatcher creates a new OS glob matcher implementation.
func NewOSGlobMatcher() *OSGlobMatcher {
	return &OSGlobMatcher{}
}

// GetGlobMatches returns all files matching the glob pattern.
func (o *OSGlobMatcher) GetGlobMatches(pattern string) ([]string, error) {
	return GetGlobMatches(pattern)
}

// PathMatch returns true if name matches the pattern.
func (o *OSGlobMatcher) PathMatch(pattern, name string) (bool, error) {
	return PathMatch(pattern, name)
}

// OSIOCopier is the default implementation that uses io.Copy.
type OSIOCopier struct{}

// NewOSIOCopier creates a new OS I/O copier implementation.
func NewOSIOCopier() *OSIOCopier {
	return &OSIOCopier{}
}

// Copy copies from src to dst until EOF or error.
func (o *OSIOCopier) Copy(dst io.Writer, src io.Reader) (written int64, err error) {
	return io.Copy(dst, src)
}
