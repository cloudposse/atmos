package vendor

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	cp "github.com/otiai10/copy"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// CopyOptions configures the copy+filter behavior for vendoring operations.
type CopyOptions struct {
	// IncludedPaths are POSIX-style glob patterns for files to include (double-star ** supported).
	IncludedPaths []string
	// ExcludedPaths are POSIX-style glob patterns for files to exclude (double-star ** supported).
	ExcludedPaths []string
}

// CopyToTarget copies files from srcDir to dstDir with include/exclude pattern filtering.
// This is the single shared code path for all vendoring operations (vendor.yaml,
// component.yaml, and source provisioning).
//
// It supports POSIX-style Globs for file names/paths (double-star ** is supported).
// https://en.wikipedia.org/wiki/Glob_(programming)
// https://github.com/bmatcuk/doublestar#patterns
func CopyToTarget(srcDir, dstDir string, opts CopyOptions) error {
	defer perf.Track(nil, "vendor.CopyToTarget")()

	copyOptions := cp.Options{
		Skip:          CreateSkipFunc(srcDir, opts.IncludedPaths, opts.ExcludedPaths),
		PreserveTimes: false,
		PreserveOwner: false,
		// Follow symlinks only when the target resolves within srcDir and
		// the resolved target passes the same skip policy as regular files.
		// This prevents symlink traversal attacks where a crafted vendor source
		// contains symlinks pointing outside the source directory or into
		// excluded directories (e.g., .git/).
		OnSymlink: createSymlinkHandler(srcDir, opts),
		// OnDirExists handles existing directories at the destination.
		// We skip .git directories from source, but if the destination already has a .git directory
		// (from a previous vendor run), we need to leave it untouched to avoid permission errors
		// on git packfiles which often have restrictive permissions.
		OnDirExists: func(src, dest string) cp.DirExistsAction {
			if filepath.Base(dest) == ".git" {
				return cp.Untouchable
			}
			return cp.Merge
		},
	}

	return cp.Copy(srcDir, dstDir, copyOptions)
}

// createSymlinkHandler returns an OnSymlink handler that validates symlink targets
// resolve within srcDir before following them, and applies the same skip policy
// (e.g., .git exclusion, exclude patterns) to the resolved target path.
// This prevents traversal attacks where a crafted vendor source contains symlinks
// pointing outside the source directory (e.g., link -> /etc/passwd) or into
// excluded directories (e.g., head.txt -> .git/HEAD).
func createSymlinkHandler(srcDir string, opts CopyOptions) func(string) cp.SymlinkAction {
	// Pre-resolve srcDir so we compare real paths consistently.
	// EvalSymlinks also resolves the absolute path.
	realSrcDir, srcErr := filepath.EvalSymlinks(srcDir)
	if srcErr != nil {
		// If we can't resolve srcDir, fall back to absolute path.
		realSrcDir, srcErr = filepath.Abs(srcDir)
	}

	return func(src string) cp.SymlinkAction {
		if srcErr != nil {
			log.Debug("Skipping symlink (cannot resolve srcDir)", "path", src, "error", srcErr)
			return cp.Skip
		}

		// Resolve the symlink target to its real path.
		resolved, err := filepath.EvalSymlinks(src)
		if err != nil {
			log.Debug("Skipping symlink (cannot resolve target)", "path", src, "error", err)
			return cp.Skip
		}

		// Ensure the resolved target is within srcDir boundaries.
		if !strings.HasPrefix(resolved, realSrcDir+string(filepath.Separator)) && resolved != realSrcDir {
			log.Debug("Skipping symlink (target outside source directory)", "path", src, "target", resolved, "srcDir", realSrcDir)
			return cp.Skip
		}

		// Apply the skip policy to the resolved target path.
		if shouldSkipSymlinkTarget(realSrcDir, resolved, src, opts) {
			return cp.Skip
		}

		return cp.Deep
	}
}

// shouldSkipSymlinkTarget checks whether the resolved symlink target should be
// skipped based on the skip policy (.git exclusion, exclude patterns).
// This catches symlinks that point into excluded directories
// (e.g., head.txt -> .git/HEAD would bypass basename checks).
func shouldSkipSymlinkTarget(realSrcDir, resolved, src string, opts CopyOptions) bool {
	relPath, err := filepath.Rel(realSrcDir, resolved)
	if err != nil {
		log.Debug("Skipping symlink (cannot compute relative path)", "path", src, "error", err)
		return true
	}

	// Check if any path component is .git.
	for _, component := range strings.Split(filepath.ToSlash(relPath), "/") {
		if component == ".git" {
			log.Debug("Skipping symlink (target resolves into .git directory)", "path", src, "target", resolved)
			return true
		}
	}

	// Check against exclude patterns if configured.
	if len(opts.ExcludedPaths) > 0 {
		normalizedRelPath := filepath.ToSlash(relPath)
		excluded, excludeErr := ShouldExcludeFile(opts.ExcludedPaths, normalizedRelPath)
		if excludeErr != nil {
			log.Debug("Skipping symlink (error checking exclude patterns)", "path", src, "error", excludeErr)
			return true
		}
		if excluded {
			log.Debug("Skipping symlink (target matches exclude pattern)", "path", src, "target", resolved, "relPath", normalizedRelPath)
			return true
		}
	}

	return false
}

// CreateSkipFunc builds a skip function for otiai10/copy that applies include/exclude patterns.
// It supports POSIX-style Globs for file names/paths (double-star ** is supported).
func CreateSkipFunc(srcDir string, includedPaths, excludedPaths []string) func(os.FileInfo, string, string) (bool, error) {
	return func(srcInfo os.FileInfo, src, dest string) (bool, error) {
		// Always skip .git directories.
		if filepath.Base(src) == ".git" {
			return true, nil
		}

		// If no patterns specified, don't skip anything else.
		if len(includedPaths) == 0 && len(excludedPaths) == 0 {
			return false, nil
		}

		// Normalize paths for cross-platform compatibility.
		normalizedSrcDir := filepath.ToSlash(srcDir)
		normalizedSrc := filepath.ToSlash(src)
		trimmedSrc := u.TrimBasePathFromPath(normalizedSrcDir+"/", normalizedSrc)

		// Check excludes first - if file matches any excluded pattern, skip it immediately.
		if len(excludedPaths) > 0 {
			excluded, err := ShouldExcludeFile(excludedPaths, trimmedSrc)
			if err != nil || excluded {
				return excluded, err
			}
		}

		// Then check includes - if specified, file must match to be included.
		if len(includedPaths) > 0 {
			// For directories, don't skip (we need to traverse them to find matching files).
			if srcInfo.IsDir() {
				return false, nil
			}
			return ShouldIncludeFile(includedPaths, trimmedSrc)
		}

		// If 'included_paths' is not provided, include all files that were not excluded.
		log.Debug("Including", "path", trimmedSrc)
		return false, nil
	}
}

// ShouldExcludeFile checks if the file matches any of the excluded patterns.
func ShouldExcludeFile(excludedPaths []string, trimmedSrc string) (bool, error) {
	for _, excludePath := range excludedPaths {
		excludePath := path.Clean(filepath.ToSlash(excludePath))
		// Match against trimmedSrc (relative path) instead of absolute path.
		// This allows simple patterns like "providers.tf" to match without needing "**/" prefix.
		excludeMatch, err := u.PathMatch(excludePath, trimmedSrc)
		if err != nil {
			// Return false (don't exclude) on error so we don't accidentally skip files
			// due to an invalid pattern. The error propagates and aborts the copy.
			return false, err
		} else if excludeMatch {
			log.Debug("Excluding file since it match any pattern from 'excluded_paths'", "excluded_paths", excludePath, "source", trimmedSrc)
			return true, nil
		}
	}
	return false, nil
}

// ShouldIncludeFile checks if the file matches any of the included patterns.
func ShouldIncludeFile(includedPaths []string, trimmedSrc string) (bool, error) {
	for _, includePath := range includedPaths {
		includePath := path.Clean(filepath.ToSlash(includePath))
		// Match against trimmedSrc (relative path) instead of absolute path.
		includeMatch, err := u.PathMatch(includePath, trimmedSrc)
		if err != nil {
			// Return false (don't skip) on error so we don't accidentally exclude files
			// due to an invalid pattern. The error propagates and aborts the copy.
			return false, err
		} else if includeMatch {
			log.Debug("Including path since it matches a pattern from 'included_paths'", "included_paths", includePath, "path", trimmedSrc)
			return false, nil
		}
	}
	// File doesn't match any included pattern, skip it.
	log.Debug("Excluding path since it does not match any pattern from 'included_paths'", "path", trimmedSrc)
	return true, nil
}
