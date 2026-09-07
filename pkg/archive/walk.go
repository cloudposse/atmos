package archive

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// packEntry is one file to add to an archive: fsPath is where to read it from
// on disk, archivePath is the forward-slash path to write it under inside the
// archive (already nested under subpath, if any).
type packEntry struct {
	fsPath      string
	archivePath string
}

// collectEntries resolves source (a file or directory) into the list of
// files to pack, applying include/exclude glob filtering (directories only)
// and nesting every archive path under subpath.
func collectEntries(source, subpath string, include, exclude []string) ([]packEntry, error) {
	defer perf.Track(nil, "archive.collectEntries")()

	if err := validateSubpath(subpath); err != nil {
		return nil, err
	}

	info, err := os.Stat(source)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrArchiveSourceNotFound).
			WithCause(err).
			WithContext("source", source).
			Err()
	}

	if !info.IsDir() {
		relPath := filepath.Base(source)
		included, err := matchesFilters(relPath, include, exclude)
		if err != nil {
			return nil, err
		}
		if !included {
			return nil, nil
		}
		archivePath := archiveJoin(filepath.ToSlash(subpath), relPath)
		return []packEntry{{fsPath: source, archivePath: archivePath}}, nil
	}

	return collectDirEntries(source, subpath, include, exclude)
}

// validateSubpath rejects a subpath that could produce an archive entry
// escaping the archive root (e.g. "../../outside"). Extracting an archive
// built with such an entry could write outside the intended extraction
// directory. Both "/" and "\" separators are checked so the rule holds
// regardless of the platform the archive is built on.
func validateSubpath(subpath string) error {
	if subpath == "" {
		return nil
	}
	normalized := strings.ReplaceAll(subpath, "\\", "/")
	if filepath.IsAbs(subpath) || strings.HasPrefix(normalized, "/") {
		return invalidSubpathError(subpath)
	}
	for _, seg := range strings.Split(normalized, "/") {
		if seg == "." || seg == ".." {
			return invalidSubpathError(subpath)
		}
	}
	return nil
}

func invalidSubpathError(subpath string) error {
	return errUtils.Build(errUtils.ErrArchiveInvalidSubpath).
		WithExplanationf("subpath %q must be a relative path with no \".\" or \"..\" segments", subpath).
		WithHint("Use a plain relative path like \"opt/nodejs\"").
		WithContext("subpath", subpath).
		Err()
}

func collectDirEntries(source, subpath string, include, exclude []string) ([]packEntry, error) {
	defer perf.Track(nil, "archive.collectDirEntries")()

	var entries []packEntry
	walkErr := filepath.WalkDir(source, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(source, p)
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(rel)

		included, err := matchesFilters(relSlash, include, exclude)
		if err != nil {
			return err
		}
		if !included {
			return nil
		}

		entries = append(entries, packEntry{
			fsPath:      p,
			archivePath: archiveJoin(filepath.ToSlash(subpath), relSlash),
		})
		return nil
	})
	if walkErr != nil {
		// matchesFilters already returns a typed ErrArchiveInvalidGlobPattern;
		// propagate it as-is instead of relabeling it as a missing source.
		if errors.Is(walkErr, errUtils.ErrArchiveInvalidGlobPattern) {
			return nil, walkErr
		}
		return nil, errUtils.Build(errUtils.ErrArchiveWalkFailed).
			WithCause(walkErr).
			WithContext("source", source).
			Err()
	}
	return entries, nil
}

// archiveJoin joins path segments with a forward slash, regardless of OS.
// Archive entry names (unlike filesystem paths) always use "/" per the
// zip/tar spec, so filepath.Join (OS-separator-aware) is the wrong tool here.
func archiveJoin(parts ...string) string {
	nonEmpty := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.Trim(p, "/")
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, "/")
}

// matchesFilters applies exclude-then-include glob matching to relPath.
// Exclude always wins; an empty include list means "include everything not
// excluded".
func matchesFilters(relPath string, include, exclude []string) (bool, error) {
	for _, pattern := range exclude {
		matched, err := u.PathMatch(pattern, relPath)
		if err != nil {
			return false, invalidGlobError(pattern, err)
		}
		if matched {
			return false, nil
		}
	}

	if len(include) == 0 {
		return true, nil
	}

	for _, pattern := range include {
		matched, err := u.PathMatch(pattern, relPath)
		if err != nil {
			return false, invalidGlobError(pattern, err)
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}

func invalidGlobError(pattern string, cause error) error {
	return errUtils.Build(errUtils.ErrArchiveInvalidGlobPattern).
		WithCause(cause).
		WithContext("pattern", pattern).
		Err()
}
