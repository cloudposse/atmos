package filesystem

import (
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/perf"
)

// CanonicalPath resolves the existing prefix of a destination that may not exist yet.
// An existing but broken symlink is an error: its destination cannot be identified.
func CanonicalPath(path string) (string, error) {
	defer perf.Track(nil, "filesystem.CanonicalPath")()

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return resolvePrefix(abs)
}

func resolvePrefix(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if !os.IsNotExist(err) {
		return resolved, err
	}
	if _, statErr := os.Lstat(path); statErr == nil || filepath.Dir(path) == path {
		return "", err
	}
	parent, err := resolvePrefix(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(path)), nil
}
