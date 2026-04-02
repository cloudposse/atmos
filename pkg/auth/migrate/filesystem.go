package migrate

import (
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/perf"
)

// OSFileSystem implements FileSystem using the real OS filesystem.
type OSFileSystem struct{}

// ReadFile reads the contents of a file at the given path.
func (fs *OSFileSystem) ReadFile(path string) ([]byte, error) {
	defer perf.Track(nil, "migrate.OSFileSystem.ReadFile")()

	return os.ReadFile(path)
}

// WriteFile writes content to a file, creating parent directories as needed.
func (fs *OSFileSystem) WriteFile(path string, data []byte, perm os.FileMode) error {
	defer perf.Track(nil, "migrate.OSFileSystem.WriteFile")()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	return os.WriteFile(path, data, perm)
}

// Exists returns true if the path exists on disk.
func (fs *OSFileSystem) Exists(path string) bool {
	defer perf.Track(nil, "migrate.OSFileSystem.Exists")()

	_, err := os.Stat(path)

	return err == nil
}

// Glob returns file paths matching the given pattern.
func (fs *OSFileSystem) Glob(pattern string) ([]string, error) {
	defer perf.Track(nil, "migrate.OSFileSystem.Glob")()

	return filepath.Glob(pattern)
}

// Remove removes a file or empty directory at the given path.
func (fs *OSFileSystem) Remove(path string) error {
	defer perf.Track(nil, "migrate.OSFileSystem.Remove")()

	return os.Remove(path)
}
