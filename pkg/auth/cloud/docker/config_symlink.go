package docker

import (
	"os"
	"path/filepath"
)

// maxConfigSymlinkHops bounds how many chained symlinks resolveConfigTarget
// follows, like the OS loop protection (a cycle never resolves).
const maxConfigSymlinkHops = 40

// resolveConfigTarget returns the file saveConfig must write so that a symlinked
// config.json stays a symlink. A fully resolvable link yields its final target.
// A dangling link (its target does not exist yet) is followed link by link, so
// the write creates the target instead of replacing the link with a regular
// file. A path that is not a symlink, or a cycle, is returned unchanged.
func resolveConfigTarget(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}

	current := path
	for range maxConfigSymlinkHops {
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			return current
		}
		dest, err := os.Readlink(current)
		if err != nil {
			return current
		}
		if !filepath.IsAbs(dest) {
			dest = filepath.Join(filepath.Dir(current), dest)
		}
		current = dest
	}
	return path
}
