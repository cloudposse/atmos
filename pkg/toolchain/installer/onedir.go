package installer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/toolchain/registry"
	"github.com/cloudposse/atmos/pkg/ui"
)

// onedirTreeName is the subdirectory within a tool's version directory that
// holds the complete, preserved archive tree for "onedir" (multi-file) packages
// such as aws-cli and nodejs/node. The registry `files:` entrypoints are exposed
// as symlinks at the version-dir root that point into this tree, so a bundled
// binary keeps its runtime siblings (shared libraries, node_modules, ...)
// alongside it.
//
// Single-binary tools do not use this directory; they remain flat in the version
// dir (see the onedir gate in extractFilesFromDir).
const onedirTreeName = ".pkg"

// entrypoint is a resolved registry files[] entry: the exposed command name and
// the template-expanded source path within the extracted archive tree.
type entrypoint struct {
	name string // The exposed command/binary name at the version-dir root.
	src  string // Template-expanded path of the file within the archive root.
}

// resolveEntrypoints expands the template in each files[].src for the tool.
func (i *Installer) resolveEntrypoints(tool *registry.Tool) ([]entrypoint, error) {
	defer perf.Track(nil, "installer.resolveEntrypoints")()

	eps := make([]entrypoint, 0, len(tool.Files))
	for _, f := range tool.Files {
		src := f.Src
		if src == "" {
			src = f.Name // Default src to name if not specified.
		}
		expanded, err := i.expandFileSrcTemplate(src, tool)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to expand file src template: %w", ErrFileOperation, err)
		}
		eps = append(eps, entrypoint{name: f.Name, src: expanded})
	}
	return eps, nil
}

// isIgnorableExtraFile reports whether a non-entrypoint file is "documentation"
// that a tool does not need at runtime (LICENSE, README, ...). Such files do not
// trigger onedir (preserved-tree) mode, so single-binary tools that merely ship
// docs alongside the binary stay on the flat layout.
func isIgnorableExtraFile(rel string) bool {
	base := strings.ToUpper(filepath.Base(rel))
	switch {
	case strings.HasPrefix(base, "LICENSE"),
		strings.HasPrefix(base, "LICENCE"),
		strings.HasPrefix(base, "README"),
		strings.HasPrefix(base, "NOTICE"),
		strings.HasPrefix(base, "CHANGELOG"),
		strings.HasPrefix(base, "AUTHORS"),
		strings.HasSuffix(base, ".MD"),
		strings.HasSuffix(base, ".TXT"):
		return true
	}
	return false
}

// shouldPreserveTree decides whether an extracted archive is a multi-file
// ("onedir") bundle that must be kept intact. It returns true when the tree
// contains files beyond the declared entrypoints and the ignorable-docs
// allowlist — i.e. potential runtime siblings (shared libraries, node_modules).
//
// This is the onedir gate. It errs toward preserving the tree (correctness for
// bundles like aws-cli/node), while keeping genuinely single-binary tools on the
// flat layout (bounded blast radius).
func shouldPreserveTree(root string, eps []entrypoint) (bool, error) {
	defer perf.Track(nil, "installer.shouldPreserveTree")()

	entrySet := make(map[string]struct{}, len(eps))
	for _, ep := range eps {
		entrySet[filepath.Clean(ep.src)] = struct{}{}
	}

	preserve := false
	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.Clean(rel)
		if _, isEntry := entrySet[rel]; isEntry {
			return nil // The entrypoint itself is expected, not "extra" payload.
		}
		if isIgnorableExtraFile(rel) {
			return nil // Docs do not count as runtime siblings.
		}
		// A non-entrypoint, non-doc file (or symlink) means the tool ships
		// runtime siblings: preserve the whole tree. Stop walking early.
		preserve = true
		return filepath.SkipAll
	})
	if walkErr != nil {
		return false, fmt.Errorf("%w: failed to inspect extracted archive: %w", ErrFileOperation, walkErr)
	}
	return preserve, nil
}

// installFlat installs the configured entrypoints directly into the version dir
// (the legacy, single-binary layout). The first file is the primary binary; any
// additional files are placed alongside it by their configured name.
func (i *Installer) installFlat(stagingDir, binaryPath string, eps []entrypoint) error {
	defer perf.Track(nil, "installer.installFlat")()

	destDir := filepath.Dir(binaryPath)
	if err := os.MkdirAll(destDir, defaultMkdirPermissions); err != nil {
		return fmt.Errorf("%w: failed to create destination directory: %w", ErrFileOperation, err)
	}

	for idx, ep := range eps {
		dst := filepath.Join(destDir, ep.name)
		if idx == 0 {
			dst = binaryPath // The first file is the primary binary.
		}
		if err := moveEntrypointFile(filepath.Join(stagingDir, ep.src), dst); err != nil {
			return err
		}
	}
	return nil
}

// moveEntrypointFile moves a single extracted file into place, handling the
// Windows convention where archives may carry a `.exe` extension the registry
// omits.
func moveEntrypointFile(src, dst string) error {
	return moveEntrypointFileForOS(src, dst, runtime.GOOS)
}

// moveEntrypointFileForOS is the OS-parameterized implementation of
// moveEntrypointFile, so the Windows `.exe` handling is testable on any platform.
func moveEntrypointFileForOS(src, dst, goos string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		if goos != "windows" {
			return fmt.Errorf("%w: file not found in archive: %s", ErrToolNotFound, src)
		}
		// Windows archives often carry a `.exe` extension the registry omits.
		srcWithExe := src + windowsExeExt
		if _, exeErr := os.Stat(srcWithExe); exeErr != nil {
			if !os.IsNotExist(exeErr) {
				return fmt.Errorf("%w: failed to stat file in archive: %s: %w", ErrFileOperation, srcWithExe, exeErr)
			}
			return fmt.Errorf("%w: file not found in archive: %s", ErrToolNotFound, src)
		}
		src = srcWithExe
		dst = ensureWindowsExeExtensionForOS(dst, goos)
	}

	if err := MoveFile(src, dst); err != nil {
		return fmt.Errorf("%w: failed to extract file %s: %w", ErrFileOperation, filepath.Base(dst), err)
	}
	if err := os.Chmod(dst, defaultMkdirPermissions); err != nil {
		return fmt.Errorf("%w: failed to make file executable: %w", ErrFileOperation, err)
	}
	return nil
}

// installOnedir preserves the complete extracted tree under <versionDir>/.pkg and
// exposes each entrypoint as a symlink at the version-dir root that points into
// the tree, so a bundled binary keeps its runtime siblings alongside it (matching
// the way the aqua CLI installs onedir packages).
func (i *Installer) installOnedir(stagingDir, binaryPath string, eps []entrypoint) error {
	defer perf.Track(nil, "installer.installOnedir")()

	versionDir := filepath.Dir(binaryPath)
	treeDir := filepath.Join(versionDir, onedirTreeName)

	// Move the whole extracted tree into place (a same-filesystem rename when the
	// staging dir is a sibling; a recursive copy otherwise).
	if err := os.RemoveAll(treeDir); err != nil {
		return fmt.Errorf("%w: failed to clear existing tree dir %s: %w", ErrFileOperation, treeDir, err)
	}
	if err := moveTree(stagingDir, treeDir); err != nil {
		return err
	}

	// Expose each entrypoint as a symlink into the preserved tree.
	for idx, ep := range eps {
		dst := filepath.Join(versionDir, ep.name)
		if idx == 0 {
			dst = binaryPath // The first file is the primary binary.
		}
		target := filepath.Join(treeDir, ep.src)
		if _, err := os.Lstat(target); err != nil {
			return fmt.Errorf("%w: entrypoint not found in archive: %s", ErrToolNotFound, ep.src)
		}
		if err := linkEntrypoint(target, dst); err != nil {
			return err
		}
	}
	return nil
}

// linkEntrypoint creates a relative symlink at linkPath pointing to target so the
// install remains relocatable.
func linkEntrypoint(target, linkPath string) error {
	if err := os.MkdirAll(filepath.Dir(linkPath), defaultMkdirPermissions); err != nil {
		return fmt.Errorf("%w: failed to create parent directory: %w", ErrFileOperation, err)
	}
	_ = os.Remove(linkPath) // Replace any existing entry (idempotent reinstall).

	rel, err := filepath.Rel(filepath.Dir(linkPath), target)
	if err != nil {
		rel = target // Fall back to an absolute link target.
	}
	if err := os.Symlink(rel, linkPath); err != nil {
		return fmt.Errorf("%w: failed to link entrypoint %s: %w", ErrFileOperation, filepath.Base(linkPath), err)
	}
	return nil
}

// extractSymlink materializes a symlink entry, preserving the (relative) link
// target from the archive. The resolved target must stay within dest so a
// malicious archive cannot link outside the extraction root.
func extractSymlink(linkPath, linkname, dest string) error {
	return extractSymlinkForOS(linkPath, linkname, dest, runtime.GOOS)
}

// extractSymlinkForOS is the OS-parameterized implementation of extractSymlink,
// so the Windows fallback (skip-with-warning) is testable on any platform.
func extractSymlinkForOS(linkPath, linkname, dest, goos string) error {
	// Validate the resolved target lexically (do NOT stat it: in a tar stream the
	// target may be extracted after the link).
	resolved := linkname
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(filepath.Dir(linkPath), linkname)
	}
	if !isSafePath(resolved, dest) {
		return fmt.Errorf("%w: illegal symlink target: %s -> %s", ErrFileOperation, linkPath, linkname)
	}

	if err := os.MkdirAll(filepath.Dir(linkPath), defaultMkdirPermissions); err != nil {
		return fmt.Errorf("%w: failed to create parent directory: %w", ErrFileOperation, err)
	}
	_ = os.Remove(linkPath)

	if err := os.Symlink(linkname, linkPath); err != nil {
		if goos == "windows" {
			// Windows onedir support is tracked separately; preserve the legacy
			// behavior of skipping links rather than failing the whole extract.
			ui.Warningf("Skipping symlink (unsupported on this platform): %s", filepath.Base(linkPath))
			return nil
		}
		return fmt.Errorf("%w: failed to create symlink %s: %w", ErrFileOperation, linkPath, err)
	}
	return nil
}

// extractHardLink materializes a tar hard-link entry. The target is relative to
// the archive root (dest). If a hard link cannot be created it falls back to
// copying the referenced file; if the target is not yet present it is skipped.
func extractHardLink(linkPath, linkname, dest string) error {
	target := filepath.Join(dest, linkname)
	if !isSafePath(target, dest) {
		return fmt.Errorf("%w: illegal hard link target: %s -> %s", ErrFileOperation, linkPath, linkname)
	}
	if err := os.MkdirAll(filepath.Dir(linkPath), defaultMkdirPermissions); err != nil {
		return fmt.Errorf("%w: failed to create parent directory: %w", ErrFileOperation, err)
	}
	_ = os.Remove(linkPath)

	if err := os.Link(target, linkPath); err == nil {
		return nil
	}

	info, statErr := os.Stat(target)
	if statErr != nil {
		ui.Warningf("Skipping hard link (target missing): %s", filepath.Base(linkPath))
		return nil
	}
	return copyRegularFile(target, linkPath, info.Mode().Perm())
}

// moveTree relocates a directory tree, preferring a fast same-filesystem rename
// and falling back to a recursive copy across filesystems.
func moveTree(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyTree(src, dst); err != nil {
		return err
	}
	if err := os.RemoveAll(src); err != nil {
		return fmt.Errorf("%w: failed to remove staging dir %s: %w", ErrFileOperation, src, err)
	}
	return nil
}

// copyTree recursively copies a directory tree, preserving regular files (with
// their mode) and symlinks.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}
		target := filepath.Join(dst, rel)
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}

		switch {
		case d.IsDir():
			return os.MkdirAll(target, info.Mode().Perm()|0o200) // Ensure the copied dir is writable.
		case info.Mode()&os.ModeSymlink != 0:
			link, linkErr := os.Readlink(path)
			if linkErr != nil {
				return linkErr
			}
			_ = os.Remove(target)
			return os.Symlink(link, target)
		default:
			return copyRegularFile(path, target, info.Mode().Perm())
		}
	})
}

// copyRegularFile copies a single file, creating parents and preserving mode.
func copyRegularFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src) // #nosec G304 -- src is an installer-extracted archive path.
	if err != nil {
		return fmt.Errorf("%w: failed to open %s: %w", ErrFileOperation, src, err)
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), defaultMkdirPermissions); err != nil {
		return fmt.Errorf("%w: failed to create parent directory: %w", ErrFileOperation, err)
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode) // #nosec G304 -- dst is within the installer version dir.
	if err != nil {
		return fmt.Errorf("%w: failed to create %s: %w", ErrFileOperation, dst, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("%w: failed to copy %s: %w", ErrFileOperation, src, err)
	}
	return nil
}

// dirExists reports whether path exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// newStagingDir creates a staging directory as a sibling of the primary binary
// (inside the tool's version dir). Extracting there keeps the subsequent tree
// move on the same filesystem, so onedir installs use a fast rename instead of a
// cross-device copy.
func newStagingDir(binaryPath string) (string, error) {
	versionDir := filepath.Dir(binaryPath)
	if err := os.MkdirAll(versionDir, defaultMkdirPermissions); err != nil {
		return "", fmt.Errorf("%w: failed to create version directory: %w", ErrFileOperation, err)
	}
	dir, err := os.MkdirTemp(versionDir, ".extract-")
	if err != nil {
		return "", fmt.Errorf("%w: failed to create staging dir: %w", ErrFileOperation, err)
	}
	return dir, nil
}
