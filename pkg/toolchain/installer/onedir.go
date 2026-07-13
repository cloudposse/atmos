package installer

import (
	"encoding/json"
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
// such as aws-cli and nodejs/node.
//
// Atmos does not create symlinks to expose these entrypoints (symlinks have
// intentionally been avoided elsewhere in Atmos, e.g. the source provisioner's
// "Copy Strategy"; see docs/prd/source-provisioner.md). Instead, each
// entrypoint's real path inside this tree is recorded in a sidecar manifest
// (see onedirManifestName) and resolved on demand by GetBinaryPath, so a
// bundled binary is invoked from its real location and keeps its runtime
// siblings (shared libraries, node_modules, ...) alongside it — without any
// symlink privilege required on Windows.
//
// The only symlinks a onedir install may contain are ones the upstream archive
// itself ships (e.g. Node's `bin/npm -> ../lib/node_modules/...` on Unix);
// those are reproduced by extractSymlink/extractHardLink during extraction,
// not created by Atmos's install/exposure logic.
//
// Single-binary tools do not use this directory; they remain flat in the version
// dir (see the onedir gate in extractFilesFromDir).
const onedirTreeName = ".pkg"

// onedirManifestName is the sidecar file recorded in a preserved onedir version
// directory. Its presence is also how callers (Uninstall, GetBinaryPath)
// distinguish a onedir install from a flat one.
const onedirManifestName = ".atmos-onedir.json"

// onedirManifest records, for a onedir install, where each entrypoint really
// lives inside the version directory.
type onedirManifest struct {
	// Entrypoints maps a configured entrypoint name (e.g. "node", "npm") to its
	// path relative to the version directory (e.g. ".pkg/node-v1.2.3-.../bin/node").
	Entrypoints map[string]string `json:"entrypoints"`
	// Primary is the entrypoint name representing the tool's main binary
	// (registry files[0]), returned when callers ask for the version's binary
	// without naming a specific entrypoint.
	Primary string `json:"primary"`
}

// writeOnedirManifest persists the manifest for a onedir install, creating the
// version directory if it does not already exist.
func writeOnedirManifest(versionDir string, manifest onedirManifest) error {
	if err := os.MkdirAll(versionDir, defaultMkdirPermissions); err != nil {
		return fmt.Errorf("%w: failed to create version directory: %w", ErrFileOperation, err)
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: failed to encode onedir manifest: %w", ErrFileOperation, err)
	}
	path := filepath.Join(versionDir, onedirManifestName)
	if err := os.WriteFile(path, data, defaultFileWritePermissions); err != nil {
		return fmt.Errorf("%w: failed to write onedir manifest %s: %w", ErrFileOperation, path, err)
	}
	return nil
}

// readOnedirManifest reads a version directory's onedir manifest, if present.
// The second return value is false for a flat (non-onedir) install or when the
// manifest is missing/unreadable.
func readOnedirManifest(versionDir string) (onedirManifest, bool) {
	data, err := os.ReadFile(filepath.Join(versionDir, onedirManifestName)) // #nosec G304 -- versionDir is the installer-managed version directory.
	if err != nil {
		return onedirManifest{}, false
	}
	var manifest onedirManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return onedirManifest{}, false
	}
	return manifest, true
}

// versionDirFromBinaryPath returns the tool's version directory given a
// resolved binary path. For a flat install the binary lives directly in the
// version dir, so this is just filepath.Dir(binaryPath). For a onedir install
// the resolved binary is nested inside the preserved tree, so this walks up
// looking for the directory that holds the onedir manifest.
func versionDirFromBinaryPath(binDir, binaryPath string) string {
	dir := filepath.Dir(binaryPath)
	for dir != binDir && dir != "." && dir != string(filepath.Separator) {
		if _, err := os.Stat(filepath.Join(dir, onedirManifestName)); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return filepath.Dir(binaryPath)
}

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
// records each entrypoint's real path in a sidecar manifest (see
// onedirManifestName), so a bundled binary is invoked from its real location and
// keeps its runtime siblings alongside it — matching the way the aqua CLI
// installs onedir packages, but without Atmos creating any symlinks of its own
// (see the package doc on onedirTreeName).
func (i *Installer) installOnedir(stagingDir, binaryPath string, eps []entrypoint) error {
	return i.installOnedirForOS(stagingDir, binaryPath, eps, runtime.GOOS)
}

// installOnedirForOS is the OS-parameterized implementation of installOnedir,
// so Windows archive entrypoint resolution is testable on every platform.
func (i *Installer) installOnedirForOS(stagingDir, binaryPath string, eps []entrypoint, goos string) error {
	defer perf.Track(nil, "installer.installOnedir")()

	versionDir := filepath.Dir(binaryPath)
	treeDir := filepath.Join(versionDir, onedirTreeName)

	// Validate every entrypoint BEFORE replacing an existing tree. A failed
	// reinstall must leave the known-good package and manifest intact rather
	// than turning a missing secondary file into a broken install.
	resolvedEntrypoints, err := resolveOnedirEntrypoints(stagingDir, eps, goos)
	if err != nil {
		return err
	}

	// Move the whole extracted tree into place (a same-filesystem rename when the
	// staging dir is a sibling; a recursive copy otherwise).
	if err := os.RemoveAll(treeDir); err != nil {
		return fmt.Errorf("%w: failed to clear existing tree dir %s: %w", ErrFileOperation, treeDir, err)
	}
	if err := moveTree(stagingDir, treeDir); err != nil {
		return err
	}

	// Record each entrypoint's real path relative to the version dir. This is
	// the ONLY exposure mechanism Atmos uses for onedir installs — no root
	// symlink is created (see the package doc on onedirTreeName).
	manifest := onedirManifest{Entrypoints: make(map[string]string, len(resolvedEntrypoints))}
	for idx, ep := range resolvedEntrypoints {
		manifest.Entrypoints[ep.name] = filepath.Join(onedirTreeName, ep.src)
		if idx == 0 {
			manifest.Primary = ep.name // The first file is the tool's main binary.
		}
	}
	return writeOnedirManifest(versionDir, manifest)
}

// resolveOnedirEntrypoints verifies that every configured entrypoint exists in
// the staged archive tree. Windows registries commonly omit the `.exe` suffix
// in files[].src, so resolve the matching archive file before the tree moves.
func resolveOnedirEntrypoints(stagingDir string, eps []entrypoint, goos string) ([]entrypoint, error) {
	resolved := make([]entrypoint, len(eps))
	for idx, ep := range eps {
		src, err := resolveOnedirEntrypointSourceForOS(stagingDir, ep.src, goos)
		if err != nil {
			return nil, err
		}
		resolved[idx] = entrypoint{name: ep.name, src: src}
	}
	return resolved, nil
}

// resolveOnedirEntrypointSourceForOS returns the archive-relative source path
// for an onedir entrypoint. On Windows, try `.exe` when the registry's source
// path omits it, matching the flat installation path's existing behavior.
func resolveOnedirEntrypointSourceForOS(stagingDir, src, goos string) (string, error) {
	path := filepath.Join(stagingDir, src)
	if _, err := os.Lstat(path); err == nil {
		return src, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("%w: failed to stat entrypoint in archive: %s: %w", ErrFileOperation, src, err)
	}

	if goos == "windows" {
		srcWithExe := src + windowsExeExt
		if _, err := os.Lstat(filepath.Join(stagingDir, srcWithExe)); err == nil {
			return srcWithExe, nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("%w: failed to stat entrypoint in archive: %s: %w", ErrFileOperation, srcWithExe, err)
		}
	}

	return "", fmt.Errorf("%w: entrypoint not found in archive: %s", ErrToolNotFound, src)
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
	// Validate the archive-supplied target and re-derive a safe one BEFORE any
	// filesystem mutation (a rejected target is always fatal, even on Windows —
	// it may be a path-traversal attempt).
	safeTarget, err := safeSymlinkTarget(linkPath, linkname, dest)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(linkPath), defaultMkdirPermissions); err != nil {
		return fmt.Errorf("%w: failed to create parent directory: %w", ErrFileOperation, err)
	}
	_ = os.Remove(linkPath)

	if err := os.Symlink(safeTarget, linkPath); err != nil {
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

// safeSymlinkTarget validates that a symlink placed at linkPath and pointing at
// linkname cannot resolve outside root, and returns a sanitized, relocatable
// (relative) target to hand to os.Symlink.
//
// Security (CodeQL go/zipslip — "arbitrary file write via archive symlinks"):
// the raw, archive-supplied linkname is NEVER passed to os.Symlink. We reject
// absolute targets outright, resolve the target relative to the link's own
// directory, and require the cleaned result to stay within root using the
// canonical filepath.Rel + ".." check. Only a target re-derived from that
// validated path is returned, so a malicious archive header cannot create a
// link that escapes the extraction root (which a later entry could then be
// written through).
func safeSymlinkTarget(linkPath, linkname, root string) (string, error) {
	if linkname == "" || filepath.IsAbs(linkname) {
		return "", fmt.Errorf("%w: illegal symlink target: %s -> %q", ErrFileOperation, linkPath, linkname)
	}

	linkDir := filepath.Dir(linkPath)
	resolved := filepath.Clean(filepath.Join(linkDir, linkname))

	rel, err := filepath.Rel(filepath.Clean(root), resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: symlink target escapes extraction root: %s -> %q", ErrFileOperation, linkPath, linkname)
	}

	// Re-derive the (relative) target from the validated, cleaned path rather
	// than reusing the raw archive string.
	safeTarget, err := filepath.Rel(linkDir, resolved)
	if err != nil {
		return "", fmt.Errorf("%w: failed to compute symlink target for %s: %w", ErrFileOperation, linkPath, err)
	}
	return safeTarget, nil
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
			// Validate + re-derive the target so the copy cannot introduce a
			// link escaping dst (same guard as archive extraction).
			safeTarget, targetErr := safeSymlinkTarget(target, link, dst)
			if targetErr != nil {
				return targetErr
			}
			_ = os.Remove(target)
			return os.Symlink(safeTarget, target)
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
