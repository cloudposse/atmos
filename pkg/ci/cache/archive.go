package cache

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// StateDirName is the per-root directory holding the cache state marker.
	// It is always excluded from the archive so the marker never travels with
	// the cached content.
	stateDirName = ".atmos-cache"

	// ArchiveDirPerm is the permission used when re-creating directories on extract.
	archiveDirPerm = 0o755
)

// archiveRoot writes a gzip-compressed tar of root to w. When includes is
// non-empty, only entries whose path (relative to root) is within one of the
// listed relative subpaths are included; otherwise the whole root is archived.
// The state directory (stateDirName) is always skipped. Unless
// allowUnsafeAuthCache is true, the default-excluded auth-cache subpaths
// (DefaultExcludedPaths) are also always skipped, regardless of includes.
//
// Tar entry names are stored relative to root with forward slashes so archives
// are portable across platforms.
func archiveRoot(w io.Writer, root string, includes []string, allowUnsafeAuthCache bool) error {
	defer perf.Track(nil, "cache.archiveRoot")()

	gw := gzip.NewWriter(w)
	tw := tar.NewWriter(gw)

	a := &archiver{tw: tw, root: root, includes: includes, allowUnsafeAuthCache: allowUnsafeAuthCache}
	walkErr := filepath.WalkDir(root, a.walk)

	if walkErr != nil {
		// Close writers best-effort; the walk error is the primary failure.
		_ = tw.Close()
		_ = gw.Close()
		return wrapErr(errUtils.ErrCacheArchiveFailed, walkErr)
	}

	if err := tw.Close(); err != nil {
		_ = gw.Close()
		return wrapErr(errUtils.ErrCacheArchiveFailed, err)
	}
	if err := gw.Close(); err != nil {
		return wrapErr(errUtils.ErrCacheArchiveFailed, err)
	}
	return nil
}

// archiver carries the state needed to archive a directory tree, keeping the
// WalkDir callback to a small signature.
type archiver struct {
	tw                   *tar.Writer
	root                 string
	includes             []string
	allowUnsafeAuthCache bool
}

// walk decides what to do with a single walked entry and writes it when it is in
// scope. It returns filepath.SkipDir to prune directories.
func (a *archiver) walk(path string, d os.DirEntry, err error) error {
	if err != nil {
		return err
	}

	rel, relErr := filepath.Rel(a.root, path)
	if relErr != nil {
		return relErr
	}
	if rel == "." {
		return nil
	}

	if handled, action := archiveSkipDecision(rel, a.includes, d.IsDir(), a.allowUnsafeAuthCache); handled {
		return action
	}

	return writeTarEntry(a.tw, path, rel, d)
}

// archiveSkipDecision decides whether a walked entry is out of scope. It returns
// handled=true with the walk action (nil to skip the single entry, or
// filepath.SkipDir to prune a directory) when the entry should not be archived.
func archiveSkipDecision(rel string, includes []string, isDir bool, allowUnsafeAuthCache bool) (bool, error) {
	// Always exclude the state directory.
	if isStateDir(rel) {
		return true, pruneAction(isDir)
	}

	// Always exclude default auth-cache subpaths, regardless of includes,
	// unless the user explicitly opted out. Evaluated before the includes
	// check so an explicit `ci.cache.paths: [auth]` cannot re-include it.
	if !allowUnsafeAuthCache && isUnderPrefix(rel, defaultExcludedPaths) {
		return true, pruneAction(isDir)
	}

	if !matchesIncludes(rel, includes) {
		return skipOutsideIncludes(rel, includes, isDir)
	}

	return false, nil
}

// isStateDir reports whether rel is the cache state marker directory or a
// path beneath it.
func isStateDir(rel string) bool {
	return rel == stateDirName || strings.HasPrefix(rel, stateDirName+string(os.PathSeparator))
}

// pruneAction returns the walk action for excluding an entry: prune the
// whole directory, or skip a single file.
func pruneAction(isDir bool) error {
	if isDir {
		return filepath.SkipDir
	}
	return nil
}

// skipOutsideIncludes handles an entry that doesn't match includes: a
// directory that cannot contain any include is pruned wholesale, a file is
// skipped, and an ancestor directory of an include is kept for further descent.
func skipOutsideIncludes(rel string, includes []string, isDir bool) (bool, error) {
	if !isDir {
		return true, nil
	}
	if !includePrefixPossible(rel, includes) {
		return true, filepath.SkipDir
	}
	return false, nil
}

// writeTarEntry writes a single directory or regular file to the tar writer.
// Symlinks and other special files are skipped.
func writeTarEntry(tw *tar.Writer, path, rel string, d os.DirEntry) error {
	info, err := d.Info()
	if err != nil {
		return err
	}

	// Only archive directories and regular files. Skip symlinks/devices to
	// avoid surprising restores and path-escape via link targets.
	if !info.IsDir() && !info.Mode().IsRegular() {
		return nil
	}

	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	// Store a portable, root-relative name (forward slashes).
	header.Name = filepath.ToSlash(rel)
	if info.IsDir() {
		header.Name += "/"
	}

	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	if info.IsDir() {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(tw, f)
	return err
}

// matchesIncludes reports whether rel (a root-relative path) is covered by the
// include list. An empty include list matches everything.
func matchesIncludes(rel string, includes []string) bool {
	if len(includes) == 0 {
		return true
	}
	return isUnderPrefix(rel, includes)
}

// isUnderPrefix reports whether rel equals or is nested under one of
// prefixes. Unlike matchesIncludes, an empty prefixes list matches nothing —
// this is used for exclusion checks, where an empty list must mean "exclude
// nothing", not "exclude everything".
func isUnderPrefix(rel string, prefixes []string) bool {
	for _, p := range prefixes {
		p = filepath.Clean(p)
		if rel == p || strings.HasPrefix(rel, p+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

// includePrefixPossible reports whether the directory rel could still contain a
// path listed in includes (i.e., rel is an ancestor of an include). Used to
// decide whether a directory walk can be pruned.
func includePrefixPossible(rel string, includes []string) bool {
	for _, inc := range includes {
		inc = filepath.Clean(inc)
		if strings.HasPrefix(inc+string(os.PathSeparator), rel+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

// extractToRoot extracts a gzip-compressed tar stream into root. Entry names
// are treated as root-relative; any entry that would escape root is rejected.
func extractToRoot(r io.Reader, root string) error {
	defer perf.Track(nil, "cache.extractToRoot")()

	gr, err := gzip.NewReader(r)
	if err != nil {
		return wrapErr(errUtils.ErrCacheExtractFailed, err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	cleanRoot := filepath.Clean(root)

	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return wrapErr(errUtils.ErrCacheExtractFailed, err)
		}

		target, err := safeJoin(cleanRoot, header.Name)
		if err != nil {
			return err
		}

		if err := extractTarEntry(tr, header, target); err != nil {
			return err
		}
	}
	return nil
}

// safeJoin joins root and a tar entry name, rejecting paths that escape root.
func safeJoin(root, name string) (string, error) {
	cleanRoot := filepath.Clean(root)
	target := filepath.Join(cleanRoot, filepath.FromSlash(name))
	// The HasPrefix guard (with trailing separator) is the canonical Zip Slip
	// barrier that CodeQL's go/zipslip query recognizes as a sanitizer. A bare
	// (non-disjunctive) guard is required so the continuing branch implies the
	// prefix holds. archiveRoot never emits an entry equal to the root itself
	// (it skips rel == "."), so rejecting a root-equal target is always safe.
	if !strings.HasPrefix(target, cleanRoot+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: entry %q escapes cache root", errUtils.ErrCacheExtractFailed, name)
	}
	return target, nil
}

// extractTarEntry writes a single tar entry (directory or regular file) to target.
func extractTarEntry(tr *tar.Reader, header *tar.Header, target string) error {
	switch header.Typeflag {
	case tar.TypeDir:
		if err := os.MkdirAll(target, archiveDirPerm); err != nil {
			return wrapErr(errUtils.ErrCacheExtractFailed, err)
		}
	case tar.TypeReg:
		return extractRegularFile(tr, header, target)
	default:
		// Skip symlinks and other special entries for safety.
	}
	return nil
}

// extractRegularFile writes a regular tar entry to target, creating parents.
func extractRegularFile(tr *tar.Reader, header *tar.Header, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), archiveDirPerm); err != nil {
		return wrapErr(errUtils.ErrCacheExtractFailed, err)
	}

	// Mask to permission bits to keep a safe, bounded FileMode conversion.
	mode := os.FileMode(header.Mode & int64(os.ModePerm)).Perm()
	if mode == 0 {
		mode = defaultFilePerm
	}

	f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return wrapErr(errUtils.ErrCacheExtractFailed, err)
	}
	// Content comes from caches we created; extraction is bounded by the archive.
	if _, err := io.Copy(f, tr); err != nil {
		_ = f.Close()
		return wrapErr(errUtils.ErrCacheExtractFailed, err)
	}
	if err := f.Close(); err != nil {
		return wrapErr(errUtils.ErrCacheExtractFailed, err)
	}
	return nil
}
