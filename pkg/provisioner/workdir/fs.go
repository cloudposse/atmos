package workdir

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/cloudposse/atmos/pkg/perf"
)

// DefaultFileSystem is the default implementation of the FileSystem interface.
type DefaultFileSystem struct{}

// NewDefaultFileSystem creates a new default file system implementation.
func NewDefaultFileSystem() *DefaultFileSystem {
	defer perf.Track(nil, "workdir.NewDefaultFileSystem")()

	return &DefaultFileSystem{}
}

// MkdirAll creates a directory along with any necessary parents.
func (f *DefaultFileSystem) MkdirAll(path string, perm fs.FileMode) error {
	defer perf.Track(nil, "workdir.DefaultFileSystem.MkdirAll")()

	return os.MkdirAll(path, perm)
}

// RemoveAll removes path and any children it contains.
func (f *DefaultFileSystem) RemoveAll(path string) error {
	defer perf.Track(nil, "workdir.DefaultFileSystem.RemoveAll")()

	return os.RemoveAll(path)
}

// Exists checks if a path exists.
func (f *DefaultFileSystem) Exists(path string) bool {
	defer perf.Track(nil, "workdir.DefaultFileSystem.Exists")()

	_, err := os.Stat(path)
	return err == nil
}

// ReadFile reads the contents of a file.
func (f *DefaultFileSystem) ReadFile(path string) ([]byte, error) {
	defer perf.Track(nil, "workdir.DefaultFileSystem.ReadFile")()

	return os.ReadFile(path)
}

// WriteFile writes data to a file with the given permissions.
func (f *DefaultFileSystem) WriteFile(path string, data []byte, perm fs.FileMode) error {
	defer perf.Track(nil, "workdir.DefaultFileSystem.WriteFile")()

	return os.WriteFile(path, data, perm)
}

// CopyDir recursively copies a directory from src to dst.
func (f *DefaultFileSystem) CopyDir(src, dst string) error {
	defer perf.Track(nil, "workdir.DefaultFileSystem.CopyDir")()

	return copyDir(src, dst)
}

// Walk walks the file tree rooted at root, calling fn for each file or directory.
func (f *DefaultFileSystem) Walk(root string, fn fs.WalkDirFunc) error {
	defer perf.Track(nil, "workdir.DefaultFileSystem.Walk")()

	return filepath.WalkDir(root, fn)
}

// Stat returns file info for the given path.
func (f *DefaultFileSystem) Stat(path string) (fs.FileInfo, error) {
	defer perf.Track(nil, "workdir.DefaultFileSystem.Stat")()

	return os.Stat(path)
}

// DefaultHasher is the default implementation of the Hasher interface.
type DefaultHasher struct{}

// NewDefaultHasher creates a new default hasher implementation.
func NewDefaultHasher() *DefaultHasher {
	defer perf.Track(nil, "workdir.NewDefaultHasher")()

	return &DefaultHasher{}
}

// HashDir computes a hash of all files in a directory.
// Files are processed in sorted order for deterministic results.
func (h *DefaultHasher) HashDir(path string) (string, error) {
	defer perf.Track(nil, "workdir.DefaultHasher.HashDir")()

	hash := sha256.New()

	// Collect all file paths first for sorted order.
	var files []string
	err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	// Sort for deterministic ordering.
	sort.Strings(files)

	// Hash each file.
	for _, file := range files {
		// Include relative path in hash for structure.
		relPath, err := filepath.Rel(path, file)
		if err != nil {
			return "", err
		}
		hash.Write([]byte(relPath))

		// Hash file contents.
		fileHash, err := h.HashFile(file)
		if err != nil {
			return "", err
		}
		hash.Write([]byte(fileHash))
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// HashFile computes a hash of a single file.
func (h *DefaultHasher) HashFile(path string) (string, error) {
	defer perf.Track(nil, "workdir.DefaultHasher.HashFile")()

	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// DefaultPathFilter is the default implementation of the PathFilter interface.
type DefaultPathFilter struct{}

// NewDefaultPathFilter creates a new default path filter implementation.
func NewDefaultPathFilter() *DefaultPathFilter {
	defer perf.Track(nil, "workdir.NewDefaultPathFilter")()

	return &DefaultPathFilter{}
}

// Match returns true if the path should be included based on include/exclude patterns.
func (f *DefaultPathFilter) Match(path string, includedPaths, excludedPaths []string) (bool, error) {
	defer perf.Track(nil, "workdir.DefaultPathFilter.Match")()

	// If no include patterns, include everything by default.
	included := len(includedPaths) == 0

	// Check include patterns.
	for _, pattern := range includedPaths {
		matched, err := filepath.Match(pattern, path)
		if err != nil {
			return false, err
		}
		if matched {
			included = true
			break
		}
	}

	// If not included, no need to check exclusions.
	if !included {
		return false, nil
	}

	// Check exclude patterns.
	for _, pattern := range excludedPaths {
		matched, err := filepath.Match(pattern, path)
		if err != nil {
			return false, err
		}
		if matched {
			return false, nil
		}
	}

	return true, nil
}
