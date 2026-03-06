package vendor

import (
	"os"
	"path/filepath"

	cp "github.com/otiai10/copy"

	"github.com/cloudposse/atmos/pkg/perf"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// CopyOptions configures the copy+filter behavior for vendoring operations.
type CopyOptions struct {
	// IncludedPaths are POSIX-style glob patterns for files to include (double-star ** supported).
	IncludedPaths []string
	// ExcludedPaths are POSIX-style glob patterns for files to exclude (double-star ** supported).
	ExcludedPaths []string
	// SourceIsLocal indicates the source is a local file (not a downloaded directory).
	SourceIsLocal bool
	// URI is the original source URI, used for local file path adjustment.
	URI string
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
		// Follow symlinks and copy their targets.
		OnSymlink: func(src string) cp.SymlinkAction { return cp.Deep },
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

	// Adjust the target path if it's a local file with no extension.
	if opts.SourceIsLocal && filepath.Ext(dstDir) == "" {
		sanitizedBase := SanitizeFileName(opts.URI)
		dstDir = filepath.Join(dstDir, sanitizedBase)
	}

	return cp.Copy(srcDir, dstDir, copyOptions)
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
		return false, nil
	}
}

// ShouldExcludeFile checks if the file matches any of the excluded patterns.
func ShouldExcludeFile(excludedPaths []string, trimmedSrc string) (bool, error) {
	for _, excludePath := range excludedPaths {
		excludePath := filepath.Clean(excludePath)
		// Match against trimmedSrc (relative path) instead of absolute path.
		// This allows simple patterns like "providers.tf" to match without needing "**/" prefix.
		excludeMatch, err := u.PathMatch(excludePath, trimmedSrc)
		if err != nil {
			return true, err
		} else if excludeMatch {
			return true, nil
		}
	}
	return false, nil
}

// ShouldIncludeFile checks if the file matches any of the included patterns.
func ShouldIncludeFile(includedPaths []string, trimmedSrc string) (bool, error) {
	for _, includePath := range includedPaths {
		includePath := filepath.Clean(includePath)
		// Match against trimmedSrc (relative path) instead of absolute path.
		includeMatch, err := u.PathMatch(includePath, trimmedSrc)
		if err != nil {
			return true, err
		} else if includeMatch {
			return false, nil
		}
	}
	// File doesn't match any included pattern, skip it.
	return true, nil
}
