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
// siblings (shared libraries, node_modules, and similar) alongside it — without
// any symlink privilege required on Windows.
//
// The only symlinks a onedir install may contain are ones the upstream archive
// itself ships (e.g. Node's `bin/npm -> ../lib/node_modules/...` on Unix).
// Those are never created straight from archive header data during extraction
// (that archive -> os.Symlink flow is a classic zip-slip write primitive).
// Instead extraction collects them (see pendingSymlink) and they are
// materialized afterward, inside the final tree, through the single validated
// sink createValidatedSymlink — not by Atmos's install/exposure logic.
//
// Single-binary tools do not use this directory; they remain flat in the version
// dir (see the onedir gate in extractFilesFromDir).
const onedirTreeName = ".pkg"

// onedirManifestName is the sidecar file recorded in a preserved onedir version
// directory. Its presence is also how callers (Uninstall, GetBinaryPath)
// distinguish a onedir install from a flat one.
const onedirManifestName = ".atmos-onedir.json"

// onedirBackupSuffix names the transient backup of an existing preserved tree
// used to make a reinstall atomic (rename aside, then restore on failure).
const onedirBackupSuffix = ".bak"

// moveTreeFunc relocates the extracted tree into place during a onedir install.
// It is a package variable solely so tests can force a move failure and exercise
// the reinstall rollback path deterministically; production always uses moveTree.
var moveTreeFunc = moveTree

// pendingSymlink is a symlink entry discovered while reading an archive, held
// aside instead of being created inline. Creating a symlink straight from
// archive header data during extraction is a zip-slip write primitive (the
// exact "arbitrary file write via archive symlinks" flow CodeQL flags), so
// onedir installs collect symlinks here and materialize them afterward — in the
// final tree, through the single validated sink createValidatedSymlink.
type pendingSymlink struct {
	// rel is the symlink's location relative to the extraction root (the raw
	// archive entry name, e.g. "node-v1.2.3/bin/npm").
	rel string
	// target is the raw link target recorded in the archive (e.g.
	// "../lib/node_modules/npm/bin/npm-cli.js"). It is validated only at
	// materialization time, never trusted at collection time.
	target string
}

// symlinkRelSet indexes collected symlinks by their cleaned, root-relative
// location, so the onedir gate and entrypoint resolver can recognize a
// symlinked path that is not yet present on disk (extraction defers symlink
// creation; see pendingSymlink).
func symlinkRelSet(symlinks []pendingSymlink) map[string]struct{} {
	set := make(map[string]struct{}, len(symlinks))
	for _, s := range symlinks {
		set[filepath.Clean(s.rel)] = struct{}{}
	}
	return set
}

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
// ("onedir") bundle that must be kept intact, versus a self-contained single
// binary that can be installed flat.
//
// This is the onedir gate. It preserves the tree when either:
//   - a declared entrypoint is itself a symlink in the archive (e.g. Node's
//     npm/npx -> ../lib/...), which only resolves with the whole tree present; or
//   - the PRIMARY entrypoint's own directory contains a non-entrypoint, non-doc
//     file — a bundled runtime or shared library the binary loads at runtime,
//     as with aws-cli's dist/libpython... (runtime siblings live next to the
//     binary).
//
// Files in unrelated subdirectories (such as completions/, manpages/, sbom/)
// and documentation next to the binary do NOT trigger onedir, so a
// self-contained single binary that merely ships docs/completions/man pages
// stays flat (bounded blast radius).
func shouldPreserveTree(root string, eps []entrypoint, symlinks []pendingSymlink) (bool, error) {
	defer perf.Track(nil, "installer.shouldPreserveTree")()

	if len(eps) == 0 {
		return false, nil
	}

	// Symlinks are collected during extraction rather than written to disk (see
	// pendingSymlink), so recognize them from the set instead of os.Lstat.
	symSet := symlinkRelSet(symlinks)

	// A symlinked entrypoint needs the whole tree preserved to resolve.
	for _, ep := range eps {
		if _, ok := symSet[filepath.Clean(ep.src)]; ok {
			return true, nil
		}
	}

	// Inspect only the directory that holds the primary entrypoint (non-recursive):
	// runtime siblings live beside the binary, not in unrelated subdirectories.
	primaryRelDir := filepath.Clean(filepath.Dir(eps[0].src))
	return hasRuntimeSibling(root, primaryRelDir, entrypointSet(eps), symSet)
}

// entrypointSet indexes entrypoints by their archive-relative path, tolerating
// the Windows `.exe` suffix that registry files[].src usually omits (e.g. src
// "tofu" matches the archive file "tofu.exe").
func entrypointSet(eps []entrypoint) map[string]struct{} {
	set := make(map[string]struct{}, 2*len(eps))
	for _, ep := range eps {
		clean := filepath.Clean(ep.src)
		set[clean] = struct{}{}
		set[clean+windowsExeExt] = struct{}{}
	}
	return set
}

// hasRuntimeSibling reports whether the primary entrypoint's own directory
// contains a runtime sibling (a non-entrypoint, non-doc file the tool loads at
// runtime), inspecting both the regular files materialized on disk and the
// symlink entries collected during extraction (which never hit disk).
func hasRuntimeSibling(root, primaryRelDir string, entrySet, symSet map[string]struct{}) (bool, error) {
	// (a) Regular-file siblings materialized on disk during extraction.
	entries, err := os.ReadDir(filepath.Join(root, primaryRelDir))
	if err != nil {
		return false, fmt.Errorf("%w: failed to inspect extracted archive: %w", ErrFileOperation, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if isRuntimeSibling(filepath.Clean(filepath.Join(primaryRelDir, e.Name())), e.Name(), entrySet) {
			return true, nil
		}
	}

	// (b) Symlink siblings, which extraction collects rather than writing to
	// disk, so they never appear in the os.ReadDir above.
	for rel := range symSet {
		if filepath.Dir(rel) != primaryRelDir {
			continue
		}
		if isRuntimeSibling(rel, filepath.Base(rel), entrySet) {
			return true, nil
		}
	}
	return false, nil
}

// isRuntimeSibling reports whether a file beside the primary entrypoint is a
// runtime sibling that forces onedir: a non-entrypoint, non-doc file the tool
// loads at runtime (a bundled library or script, say). Declared entrypoints
// (and their Windows `.exe` form) and documentation do not count.
func isRuntimeSibling(rel, base string, entrySet map[string]struct{}) bool {
	if _, isEntry := entrySet[rel]; isEntry {
		return false // A declared entrypoint (or its .exe form) is not "extra".
	}
	if isIgnorableExtraFile(base) {
		return false // Docs/man/completions do not count as runtime siblings.
	}
	return true
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
func (i *Installer) installOnedir(stagingDir, binaryPath string, eps []entrypoint, symlinks []pendingSymlink) error {
	return i.installOnedirForOS(stagingDir, binaryPath, eps, symlinks, runtime.GOOS)
}

// installOnedirForOS is the OS-parameterized implementation of installOnedir,
// so Windows archive entrypoint resolution is testable on every platform.
func (i *Installer) installOnedirForOS(stagingDir, binaryPath string, eps []entrypoint, symlinks []pendingSymlink, goos string) error {
	defer perf.Track(nil, "installer.installOnedir")()

	versionDir := filepath.Dir(binaryPath)
	treeDir := filepath.Join(versionDir, onedirTreeName)

	// Validate every entrypoint BEFORE replacing an existing tree. A failed
	// reinstall must leave the known-good package and manifest intact rather
	// than turning a missing secondary file into a broken install. Symlinked
	// entrypoints are recognized from the collected set, since extraction does
	// not write them to disk (see pendingSymlink).
	resolvedEntrypoints, err := resolveOnedirEntrypoints(stagingDir, eps, symlinks, goos)
	if err != nil {
		return err
	}

	// Atomically replace any existing tree. Rename the current tree aside first
	// (rather than deleting it outright) so that if the move or symlink
	// materialization fails partway (disk full, interrupted cross-device copy,
	// escaping symlink, ...) the known-good install can be restored instead of
	// being left with no tree and a stale manifest.
	backupDir := treeDir + onedirBackupSuffix
	_ = os.RemoveAll(backupDir) // Clear any residue from an interrupted prior run.
	hadExisting := dirExists(treeDir)
	if hadExisting {
		if err := os.Rename(treeDir, backupDir); err != nil {
			return fmt.Errorf("%w: failed to back up existing tree dir %s: %w", ErrFileOperation, treeDir, err)
		}
	}
	if err := populateOnedirTree(stagingDir, treeDir, symlinks, goos); err != nil {
		_ = os.RemoveAll(treeDir) // Discard the partial tree.
		if hadExisting {
			_ = os.Rename(backupDir, treeDir) // Restore the known-good tree.
		}
		return err
	}
	if hadExisting {
		_ = os.RemoveAll(backupDir)
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

// populateOnedirTree moves the extracted regular-file tree into its final
// location and then materializes the archive's own symlinks inside it. Symlinks
// are recreated only after the move so they go through the single validated
// sink (createValidatedSymlink) in the final tree, never straight from archive
// header data during extraction.
func populateOnedirTree(stagingDir, treeDir string, symlinks []pendingSymlink, goos string) error {
	if err := moveTreeFunc(stagingDir, treeDir); err != nil {
		return err
	}
	return materializeSymlinks(treeDir, symlinks, goos)
}

// resolveOnedirEntrypoints verifies that every configured entrypoint exists in
// the staged archive tree. Windows registries commonly omit the `.exe` suffix
// in files[].src, so resolve the matching archive file before the tree moves.
// A symlinked entrypoint is not yet on disk (extraction defers symlink
// creation; see pendingSymlink), so it is recognized from the collected set.
func resolveOnedirEntrypoints(stagingDir string, eps []entrypoint, symlinks []pendingSymlink, goos string) ([]entrypoint, error) {
	symSet := symlinkRelSet(symlinks)
	resolved := make([]entrypoint, len(eps))
	for idx, ep := range eps {
		src, err := resolveOnedirEntrypointSourceForOS(stagingDir, ep.src, symSet, goos)
		if err != nil {
			return nil, err
		}
		resolved[idx] = entrypoint{name: ep.name, src: src}
	}
	return resolved, nil
}

// resolveOnedirEntrypointSourceForOS returns the archive-relative source path
// for an onedir entrypoint. A path is present if it was extracted to disk or
// collected as a pending symlink (symSet). On Windows, try `.exe` when the
// registry's source path omits it, matching the flat installation path's
// existing behavior.
func resolveOnedirEntrypointSourceForOS(stagingDir, src string, symSet map[string]struct{}, goos string) (string, error) {
	if present, err := onedirSourcePresent(stagingDir, src, symSet); err != nil {
		return "", err
	} else if present {
		return src, nil
	}

	if goos == "windows" {
		srcWithExe := src + windowsExeExt
		if present, err := onedirSourcePresent(stagingDir, srcWithExe, symSet); err != nil {
			return "", err
		} else if present {
			return srcWithExe, nil
		}
	}

	return "", fmt.Errorf("%w: entrypoint not found in archive: %s", ErrToolNotFound, src)
}

// onedirSourcePresent reports whether an archive-relative source path was
// materialized on disk or collected as a pending symlink.
func onedirSourcePresent(stagingDir, src string, symSet map[string]struct{}) (bool, error) {
	if _, ok := symSet[filepath.Clean(src)]; ok {
		return true, nil
	}
	if _, err := os.Lstat(filepath.Join(stagingDir, src)); err == nil {
		return true, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("%w: failed to stat entrypoint in archive: %s: %w", ErrFileOperation, src, err)
	}
	return false, nil
}

// materializeSymlinks recreates an archive's own symlinks inside the final
// onedir tree, after the regular files have been moved into place. Every link
// is created through createValidatedSymlink — the single validated os.Symlink
// sink — so a symlink is never written straight from archive header data during
// extraction (that archive -> os.Symlink flow is a zip-slip write primitive).
func materializeSymlinks(root string, symlinks []pendingSymlink, goos string) error {
	for _, s := range symlinks {
		if err := createValidatedSymlink(root, filepath.Join(root, s.rel), s.target, goos); err != nil {
			return err
		}
	}
	return nil
}

// createValidatedSymlink is the single symlink-creation sink in the installer.
// It reproduces an archive's own symlink inside the preserved onedir tree, with
// both archive-controlled inputs sanitized INLINE, immediately before the
// os.Symlink call:
//   - the symlink's LOCATION (linkPath, from the entry name) must stay within root, and
//   - the symlink's TARGET, resolved relative to the link's own directory, must
//     stay within root (absolute targets are rejected outright).
//
// Security (CodeQL go/zipslip — "arbitrary file write via archive symlinks"):
// os.Symlink receives the SAME cleaned, absolute path (resolved) that the
// containment check guards, in this function's own body — not a value
// re-derived after the check, and not a value guarded in a different function.
// That intraprocedural "guard, then pass the guarded value" shape is what the
// analysis credits, and it is why extraction defers symlink creation to this
// sink instead of calling os.Symlink from the archive-reading loop.
//
// The link target is therefore absolute, so a onedir tree is pinned to its
// version directory: it is reinstalled rather than relocated (documented in the
// install guide's "How Tools Are Installed on Disk").
func createValidatedSymlink(root, linkPath, linkname, goos string) error {
	cleanRoot := filepath.Clean(root)

	// (1) The symlink's own location must stay within root.
	cleanLinkPath := filepath.Clean(linkPath)
	if cleanLinkPath != cleanRoot && !strings.HasPrefix(cleanLinkPath, cleanRoot+string(os.PathSeparator)) {
		return fmt.Errorf("%w: symlink path escapes root: %s", ErrFileOperation, linkPath)
	}

	// (2) The target must be relative and, once resolved against the link's own
	// directory, stay within root.
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

	// os.Symlink receives the guarded absolute `resolved` value directly.
	if err := os.Symlink(resolved, cleanLinkPath); err != nil {
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
			// Reproduce the link through the single validated symlink sink so a
			// copied tree cannot introduce a link escaping dst. (During a onedir
			// install the staging tree has no symlinks — they are collected and
			// materialized separately — so this is a defensive path for the
			// generic cross-device copy fallback.)
			return createValidatedSymlink(dst, target, link, runtime.GOOS)
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
