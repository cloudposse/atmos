package docker

import "github.com/cloudposse/atmos/pkg/filesystem"

// resolveConfigTarget returns the file saveConfig must write so that a symlinked
// config.json stays a symlink. A fully resolvable link yields its final target.
// A dangling link (its target does not exist yet) is followed link by link, so
// the write creates the target instead of replacing the link with a regular
// file. A path that is not a symlink, or a cycle, is returned unchanged. The
// resolution is shared with the other files Atmos writes for other tools (see
// filesystem.ResolveSymlinkTarget).
func resolveConfigTarget(path string) string {
	return filesystem.ResolveSymlinkTarget(path)
}
