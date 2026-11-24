package filesystem

import (
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/perf"
)

// EvalSymlinksBestEffort resolves symlinks in path even when the full path
// does not exist, by resolving the deepest existing ancestor and re-appending
// the non-existent remainder. The plain filepath.EvalSymlinks errors on
// missing paths, and changed or deleted files routinely do not exist.
// A path with no resolvable ancestor is returned unchanged.
func EvalSymlinksBestEffort(path string) string {
	defer perf.Track(nil, "filesystem.EvalSymlinksBestEffort")()

	remainder := ""
	current := path
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			return filepath.Join(resolved, remainder)
		}
		parent := filepath.Dir(current)
		if parent == current {
			// Nothing along the path resolves — return the input unchanged.
			return path
		}
		remainder = filepath.Join(filepath.Base(current), remainder)
		current = parent
	}
}
