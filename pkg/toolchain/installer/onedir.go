package installer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/toolchain/registry"
)

// onedirTreeName holds a preserved multi-file package tree. Entrypoints resolve
// through its manifest; archive symlinks are materialized only after extraction.
const onedirTreeName = ".pkg"

// onedirManifestName identifies a preserved onedir install.
const onedirManifestName = ".atmos-onedir.json"

// onedirBackupSuffix identifies the transient reinstall backup tree.
const onedirBackupSuffix = ".bak"

// moveTreeFunc allows tests to force a move failure.
var moveTreeFunc = moveTree

// pendingSymlink is an archive symlink materialized after extraction.
type pendingSymlink struct {
	rel    string
	target string
}

// pendingHardLink records a tar hard link until its target has been extracted.
type pendingHardLink struct {
	rel    string
	target string
}

// deferredEntries carries the archive entries whose creation is deferred until
// after regular files are extracted. They travel together through the onedir
// install so a hard link whose target is a symlink resolves.
type deferredEntries struct {
	symlinks  []pendingSymlink
	hardLinks []pendingHardLink
}

// symlinkRelSet indexes deferred symlinks by cleaned relative path.
func symlinkRelSet(symlinks []pendingSymlink) map[string]struct{} {
	set := make(map[string]struct{}, len(symlinks))
	for _, s := range symlinks {
		set[filepath.Clean(s.rel)] = struct{}{}
	}
	return set
}

// hardLinkRelSet indexes deferred hard links by cleaned relative path.
func hardLinkRelSet(hardLinks []pendingHardLink) map[string]struct{} {
	set := make(map[string]struct{}, len(hardLinks))
	for _, h := range hardLinks {
		set[filepath.Clean(h.rel)] = struct{}{}
	}
	return set
}

// deferredRelSet unions the symlink and hard-link rel sets. Both are collected
// during extraction rather than written to disk, so callers that need to see
// every deferred entry (the onedir gate, entrypoint resolution) use this.
func deferredRelSet(symlinks []pendingSymlink, hardLinks []pendingHardLink) map[string]struct{} {
	set := symlinkRelSet(symlinks)
	for rel := range hardLinkRelSet(hardLinks) {
		set[rel] = struct{}{}
	}
	return set
}

// onedirManifest records onedir entrypoint paths.
type onedirManifest struct {
	Entrypoints map[string]string `json:"entrypoints"`
	Primary     string            `json:"primary"`
}

// writeOnedirManifest persists an onedir manifest.
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

// readOnedirManifest returns the manifest when present and valid.
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

// versionDirFromBinaryPath finds the flat or onedir version directory.
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

// entrypoint is a resolved registry files[] entry.
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

// isIgnorableExtraFile reports whether a file is documentation.
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

// shouldPreserveTree keeps archives with symlinked entrypoints or runtime files.
func shouldPreserveTree(root string, eps []entrypoint, symlinks []pendingSymlink, hardLinks []pendingHardLink) (bool, error) {
	defer perf.Track(nil, "installer.shouldPreserveTree")()

	if len(eps) == 0 {
		return false, nil
	}

	// A symlinked entrypoint needs the whole tree to resolve. Hard-linked
	// entrypoints do not (they become real files), so only symlinks count here.
	symSet := symlinkRelSet(symlinks)
	for _, ep := range eps {
		if _, ok := symSet[filepath.Clean(ep.src)]; ok {
			return true, nil
		}
	}

	primaryRelDir := filepath.Clean(filepath.Dir(eps[0].src))
	if preserve, err := hasRuntimeDirectory(root, primaryRelDir); err != nil || preserve {
		return preserve, err
	}
	// Deferred symlinks and hard links are not on disk yet, so include both when
	// scanning for runtime siblings beside the primary binary.
	deferred := deferredRelSet(symlinks, hardLinks)
	return hasRuntimeSibling(root, primaryRelDir, entrypointSet(eps), deferred)
}

// hasRuntimeDirectory detects standard shared-library directories near a binary.
func hasRuntimeDirectory(root, primaryRelDir string) (bool, error) {
	dirs := []string{primaryRelDir}
	if filepath.Base(primaryRelDir) == "bin" {
		dirs = append(dirs, filepath.Dir(primaryRelDir))
	}
	for _, dir := range dirs {
		for _, name := range []string{"lib", "lib64"} {
			info, err := os.Stat(filepath.Join(root, dir, name))
			if err == nil && info.IsDir() {
				return true, nil
			}
			if err != nil && !os.IsNotExist(err) {
				return false, fmt.Errorf("%w: failed to inspect extracted archive: %w", ErrFileOperation, err)
			}
		}
	}
	return false, nil
}

// entrypointSet includes Windows .exe variants.
func entrypointSet(eps []entrypoint) map[string]struct{} {
	set := make(map[string]struct{}, 2*len(eps))
	for _, ep := range eps {
		clean := filepath.Clean(ep.src)
		set[clean] = struct{}{}
		set[clean+windowsExeExt] = struct{}{}
	}
	return set
}

// hasRuntimeSibling checks regular and deferred (symlink/hard-link) siblings.
func hasRuntimeSibling(root, primaryRelDir string, entrySet, deferredSet map[string]struct{}) (bool, error) {
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

	for rel := range deferredSet {
		if filepath.Dir(rel) != primaryRelDir {
			continue
		}
		if isRuntimeSibling(rel, filepath.Base(rel), entrySet) {
			return true, nil
		}
	}
	return false, nil
}

// isRuntimeSibling excludes entrypoints and documentation.
func isRuntimeSibling(rel, base string, entrySet map[string]struct{}) bool {
	if _, isEntry := entrySet[rel]; isEntry {
		return false
	}
	if isIgnorableExtraFile(base) {
		return false
	}
	return true
}

// installFlat installs entrypoints in the version directory.
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

// moveEntrypointFile moves one extracted entrypoint.
func moveEntrypointFile(src, dst string) error {
	return moveEntrypointFileForOS(src, dst, runtime.GOOS)
}

// moveEntrypointFileForOS supports testable Windows .exe handling.
func moveEntrypointFileForOS(src, dst, goos string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		if goos != "windows" {
			return fmt.Errorf("%w: file not found in archive: %s", ErrToolNotFound, src)
		}
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

// installOnedir preserves the archive tree and writes its manifest.
func (i *Installer) installOnedir(stagingDir, binaryPath string, eps []entrypoint, symlinks []pendingSymlink, hardLinks []pendingHardLink) error {
	return i.installOnedirForOS(stagingDir, binaryPath, eps, deferredEntries{symlinks: symlinks, hardLinks: hardLinks}, runtime.GOOS)
}

// installOnedirForOS supports testable Windows entrypoint resolution.
func (i *Installer) installOnedirForOS(stagingDir, binaryPath string, eps []entrypoint, deferred deferredEntries, goos string) error {
	defer perf.Track(nil, "installer.installOnedir")()

	versionDir := filepath.Dir(binaryPath)
	treeDir := filepath.Join(versionDir, onedirTreeName)

	// Validate before replacing an existing install.
	resolvedEntrypoints, err := resolveOnedirEntrypoints(stagingDir, eps, deferred, goos)
	if err != nil {
		return err
	}

	// Rename aside so a failed move, materialization, or manifest write can roll
	// back to the known-good install.
	backupDir := treeDir + onedirBackupSuffix
	_ = os.RemoveAll(backupDir)
	hadExisting := dirExists(treeDir)
	if hadExisting {
		if err := os.Rename(treeDir, backupDir); err != nil {
			return fmt.Errorf("%w: failed to back up existing tree dir %s: %w", ErrFileOperation, treeDir, err)
		}
	}

	if err := i.finishOnedirInstall(stagingDir, versionDir, treeDir, resolvedEntrypoints, deferred); err != nil {
		_ = os.RemoveAll(treeDir)
		if hadExisting {
			_ = os.Rename(backupDir, treeDir)
		}
		return err
	}
	if hadExisting {
		_ = os.RemoveAll(backupDir)
	}
	return nil
}

// finishOnedirInstall populates the tree and writes the manifest. The manifest
// is written before the caller removes the backup, so a manifest-write failure
// still rolls back to the known-good install rather than leaving a tree with a
// stale or missing manifest.
func (i *Installer) finishOnedirInstall(stagingDir, versionDir, treeDir string, eps []entrypoint, deferred deferredEntries) error {
	if err := populateOnedirTree(stagingDir, treeDir, deferred); err != nil {
		return err
	}

	manifest := onedirManifest{Entrypoints: make(map[string]string, len(eps))}
	for idx, ep := range eps {
		manifest.Entrypoints[ep.name] = filepath.Join(onedirTreeName, ep.src)
		if idx == 0 {
			manifest.Primary = ep.name
		}
	}
	return writeOnedirManifest(versionDir, manifest)
}

// populateOnedirTree moves files, then materializes archive symlinks and hard
// links. Hard links are created after symlinks so a hard link whose target is a
// symlink entry resolves.
func populateOnedirTree(stagingDir, treeDir string, deferred deferredEntries) error {
	if err := moveTreeFunc(stagingDir, treeDir); err != nil {
		return err
	}
	if err := materializeSymlinks(treeDir, deferred.symlinks); err != nil {
		return err
	}
	return materializeHardLinks(treeDir, deferred.hardLinks)
}

// resolveOnedirEntrypoints validates staged entrypoints. Deferred symlinks and
// hard links are not on disk yet, so both count as present.
func resolveOnedirEntrypoints(stagingDir string, eps []entrypoint, deferred deferredEntries, goos string) ([]entrypoint, error) {
	deferredSet := deferredRelSet(deferred.symlinks, deferred.hardLinks)
	resolved := make([]entrypoint, len(eps))
	for idx, ep := range eps {
		src, err := resolveOnedirEntrypointSourceForOS(stagingDir, ep.src, deferredSet, goos)
		if err != nil {
			return nil, err
		}
		resolved[idx] = entrypoint{name: ep.name, src: src}
	}
	return resolved, nil
}

// resolveOnedirEntrypointSourceForOS resolves a file, deferred symlink, or .exe.
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

// onedirSourcePresent checks staged files and deferred symlinks.
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

// materializeSymlinks creates archive symlinks after the tree is moved.
func materializeSymlinks(root string, symlinks []pendingSymlink) error {
	for _, s := range symlinks {
		if err := createValidatedSymlink(root, filepath.Join(root, s.rel), s.target); err != nil {
			return err
		}
	}
	return nil
}

// createValidatedSymlink creates an in-tree archive symlink.
// It validates both link and target paths immediately before os.Symlink.
func createValidatedSymlink(root, linkPath, linkname string) error {
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

	if err := os.Symlink(resolved, cleanLinkPath); err != nil {
		return fmt.Errorf("%w: failed to create symlink %s: %w", ErrFileOperation, linkPath, err)
	}
	return nil
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
