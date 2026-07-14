package installer

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// materializeSymlinks creates archive symlinks after the tree is moved.
func materializeSymlinks(root string, symlinks []pendingSymlink) error {
	for _, s := range symlinks {
		if err := createValidatedSymlink(root, filepath.Join(root, s.rel), s.target); err != nil {
			return err
		}
	}
	return nil
}

// symlinkFunc is the os.Symlink seam, a package variable solely so tests can
// force a symlink-creation failure and exercise the Windows fallback path
// deterministically on any platform; production always uses os.Symlink.
var symlinkFunc = os.Symlink

// createValidatedSymlink creates an in-tree archive symlink.
// It validates both link and target paths immediately before os.Symlink.
func createValidatedSymlink(root, linkPath, linkname string) error {
	return createValidatedSymlinkForOS(root, linkPath, linkname, runtime.GOOS)
}

// createValidatedSymlinkForOS is the OS-parameterized implementation, so the
// Windows fallback is testable on any platform.
func createValidatedSymlinkForOS(root, linkPath, linkname, goos string) error {
	cleanRoot := filepath.Clean(root)

	cleanLinkPath := filepath.Clean(linkPath)
	if cleanLinkPath != cleanRoot && !strings.HasPrefix(cleanLinkPath, cleanRoot+string(os.PathSeparator)) {
		return fmt.Errorf("%w: symlink path escapes root: %s", ErrFileOperation, linkPath)
	}

	if linkname == "" || filepath.IsAbs(linkname) {
		return fmt.Errorf("%w: illegal symlink target: %s -> %q", ErrFileOperation, linkPath, linkname)
	}
	resolved := filepath.Clean(filepath.Join(filepath.Dir(cleanLinkPath), linkname))
	if resolved != cleanRoot && !strings.HasPrefix(resolved, cleanRoot+string(os.PathSeparator)) {
		return fmt.Errorf("%w: symlink target escapes root: %s -> %q", ErrFileOperation, linkPath, linkname)
	}

	if err := os.MkdirAll(filepath.Dir(cleanLinkPath), defaultMkdirPermissions); err != nil {
		return fmt.Errorf("%w: failed to create parent directory: %w", ErrFileOperation, err)
	}
	_ = os.Remove(cleanLinkPath)

	if err := symlinkFunc(resolved, cleanLinkPath); err == nil {
		return nil
	} else if goos != "windows" {
		return fmt.Errorf("%w: failed to create symlink %s: %w", ErrFileOperation, linkPath, err)
	}

	// Windows without symlink privilege (Developer Mode / admin): fall back to a
	// hard link, then a copy, for a regular-file target so common onedir
	// packages install without elevation. Directory symlinks are unsupported.
	return windowsSymlinkFallback(resolved, cleanLinkPath, linkPath)
}

// windowsSymlinkFallback reproduces a regular-file symlink target as a hard link
// (or a copy if hard-linking fails) when os.Symlink is unavailable on Windows.
// Directory targets and missing targets are reported as errors.
func windowsSymlinkFallback(resolvedTarget, linkPath, originalLinkPath string) error {
	info, err := os.Stat(resolvedTarget)
	if err != nil {
		return fmt.Errorf("%w: failed to create symlink %s: %w", ErrFileOperation, originalLinkPath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%w: cannot reproduce directory symlink without privilege: %s", ErrFileOperation, originalLinkPath)
	}
	if err := os.Link(resolvedTarget, linkPath); err == nil {
		return nil
	}
	return copyRegularFile(resolvedTarget, linkPath, info.Mode().Perm())
}

// materializeHardLinks creates deferred tar hard links after extraction.
func materializeHardLinks(root string, links []pendingHardLink) error {
	remaining := links
	for len(remaining) > 0 {
		next := make([]pendingHardLink, 0, len(remaining))
		for _, link := range remaining {
			err := extractHardLink(filepath.Join(root, link.rel), link.target, root)
			if errors.Is(err, ErrToolNotFound) {
				next = append(next, link)
				continue
			}
			if err != nil {
				return err
			}
		}
		if len(next) == len(remaining) {
			return fmt.Errorf("%w: hard link target not found: %s", ErrToolNotFound, next[0].target)
		}
		remaining = next
	}
	return nil
}

// extractHardLink materializes a tar hard link relative to root.
func extractHardLink(linkPath, linkname, dest string) error {
	// linkname is an archive-relative target. Reject absolute targets explicitly
	// (filepath.Join's handling of absolute paths differs across platforms), just
	// as createValidatedSymlink does for symlinks.
	if filepath.IsAbs(linkname) {
		return fmt.Errorf("%w: absolute hard link target not allowed: %s -> %s", ErrFileOperation, linkPath, linkname)
	}
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
		return fmt.Errorf("%w: hard link target not found: %s", ErrToolNotFound, linkname)
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
			// Reproduce the link through the single validated symlink sink so a
			// copied tree cannot introduce a link escaping dst. (During a onedir
			// install the staging tree has no symlinks — they are collected and
			// materialized separately — so this is a defensive path for the
			// generic cross-device copy fallback.)
			return createValidatedSymlink(dst, target, link)
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
