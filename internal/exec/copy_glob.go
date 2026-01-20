package exec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	cp "github.com/otiai10/copy" // Using the optimized copy library when no filtering is required.
)

// Named constants to avoid literal duplication.
const (
	logKeyPath        = "path"
	logKeyError       = "error"
	logKeyPattern     = "pattern"
	shallowCopySuffix = "/*"

	// finalTargetKey is used as a logging key for the final target.
	finalTargetKey = "finalTarget"

	// sourceKey is used as a logging key for the source.
	sourceKey = "source"
)

// PrefixCopyContext groups parameters for prefix-based copy operations.
type PrefixCopyContext struct {
	SrcDir     string
	DstDir     string
	GlobalBase string
	Prefix     string
	Excluded   []string
}

// CopyContext groups parameters for directory copy operations.
type CopyContext struct {
	SrcDir   string
	DstDir   string
	BaseDir  string
	Excluded []string
	Included []string
}

// FileCopier provides file copying operations with injectable dependencies for testing.
type FileCopier struct {
	fs     filesystem.FileSystem
	glob   filesystem.GlobMatcher
	ioCopy filesystem.IOCopier
}

// NewFileCopier creates a new FileCopier with the given dependencies.
func NewFileCopier(fs filesystem.FileSystem, glob filesystem.GlobMatcher, ioCopy filesystem.IOCopier) *FileCopier {
	return &FileCopier{
		fs:     fs,
		glob:   glob,
		ioCopy: ioCopy,
	}
}

// defaultFileCopier is the default instance used by package-level functions.
var defaultFileCopier = NewFileCopier(
	filesystem.NewOSFileSystem(),
	filesystem.NewOSGlobMatcher(),
	filesystem.NewOSIOCopier(),
)

// copyFile copies a single file from src to dst while preserving file permissions.
// This is a convenience wrapper around the default FileCopier.
func copyFile(src, dst string) error {
	return defaultFileCopier.copyFile(src, dst)
}

// copyFile copies a single file from src to dst while preserving file permissions.
func (fc *FileCopier) copyFile(src, dst string) error {
	sourceFile, err := fc.fs.Open(src)
	if err != nil {
		return errors.Join(errUtils.ErrOpenFile, fmt.Errorf("opening source file %q: %w", src, err))
	}
	defer sourceFile.Close()
	if err := fc.fs.MkdirAll(filepath.Dir(dst), os.ModePerm); err != nil {
		return errors.Join(errUtils.ErrCreateDirectory, fmt.Errorf("creating destination directory for %q: %w", dst, err))
	}

	destinationFile, err := fc.fs.Create(dst)
	if err != nil {
		return errors.Join(errUtils.ErrOpenFile, fmt.Errorf("creating destination file %q: %w", dst, err))
	}
	defer destinationFile.Close()

	// COVERAGE NOTE: Now testable via IOCopier mock - see copy_glob_error_paths_test.go.
	if _, err := fc.ioCopy.Copy(destinationFile, sourceFile); err != nil {
		return fmt.Errorf("copying content from %q to %q: %w", src, dst, err)
	}

	// COVERAGE NOTE: Now testable via FileSystem mock - see copy_glob_error_paths_test.go.
	info, err := fc.fs.Stat(src)
	if err != nil {
		return errors.Join(errUtils.ErrStatFile, fmt.Errorf("getting file info for %q: %w", src, err))
	}
	// COVERAGE NOTE: Now testable via FileSystem mock - see copy_glob_error_paths_test.go.
	if err := fc.fs.Chmod(dst, info.Mode()); err != nil {
		return errors.Join(errUtils.ErrSetPermissions, fmt.Errorf("setting permissions on %q: %w", dst, err))
	}
	return nil
}

// shouldExcludePath checks exclusion patterns for a given relative path and file info.
func shouldExcludePath(info os.FileInfo, relPath string, excluded []string) bool {
	return defaultFileCopier.shouldExcludePath(info, relPath, excluded)
}

// shouldExcludePath checks exclusion patterns for a given relative path and file info.
func (fc *FileCopier) shouldExcludePath(info os.FileInfo, relPath string, excluded []string) bool {
	for _, pattern := range excluded {
		// Check plain relative path.
		matched, err := fc.glob.PathMatch(pattern, relPath)
		if err != nil {
			log.Debug("Error matching exclusion pattern", logKeyPath, relPath, logKeyError, err)
			continue
		}
		if matched {
			log.Debug("Excluding path due to exclusion pattern (plain match)", logKeyPath, relPath, logKeyPattern, pattern)
			return true
		}
		// If a directory, also check with a trailing slash.
		if info.IsDir() {
			matched, err = fc.glob.PathMatch(pattern, relPath+"/")
			if err != nil {
				log.Debug("Error matching exclusion pattern with trailing slash", logKeyPattern, pattern, logKeyPath, relPath+"/", logKeyError, err)
				continue
			}
			if matched {
				log.Debug("Excluding directory due to exclusion pattern (with trailing slash)", logKeyPath, relPath+"/", logKeyPattern, pattern)
				return true
			}
		}
	}
	return false
}

// shouldIncludePath checks whether a file should be included based on inclusion patterns.
func shouldIncludePath(info os.FileInfo, relPath string, included []string) bool {
	return defaultFileCopier.shouldIncludePath(info, relPath, included)
}

// shouldIncludePath checks whether a file should be included based on inclusion patterns.
func (fc *FileCopier) shouldIncludePath(info os.FileInfo, relPath string, included []string) bool {
	// Directories are generally handled by recursion.
	if len(included) == 0 || info.IsDir() {
		return true
	}
	for _, pattern := range included {
		matched, err := fc.glob.PathMatch(pattern, relPath)
		if err != nil {
			log.Debug("Error matching inclusion pattern", logKeyPattern, pattern, logKeyPath, relPath, logKeyError, err)
			continue
		}
		if matched {
			log.Debug("Including path due to inclusion pattern", logKeyPath, relPath, logKeyPattern, pattern)
			return true
		}
	}
	log.Debug("Excluding path because it does not match any inclusion pattern", logKeyPath, relPath)
	return false
}

// shouldSkipEntry determines whether to skip a file/directory based on its relative path to baseDir.
func shouldSkipEntry(info os.FileInfo, srcPath, baseDir string, excluded, included []string) bool {
	if info.Name() == ".git" {
		return true
	}
	relPath, err := filepath.Rel(baseDir, srcPath)
	if err != nil {
		log.Debug("Error computing relative path", "srcPath", srcPath, logKeyError, err)
		return true // treat error as a signal to skip
	}
	relPath = filepath.ToSlash(relPath)
	if shouldExcludePath(info, relPath, excluded) {
		return true
	}
	if !shouldIncludePath(info, relPath, included) {
		return true
	}
	return false
}

// processDirEntry handles a single directory entry for copyDirRecursive.
func processDirEntry(entry os.DirEntry, ctx *CopyContext) error {
	srcPath := filepath.Join(ctx.SrcDir, entry.Name())
	dstPath := filepath.Join(ctx.DstDir, entry.Name())

	info, err := entry.Info()
	if err != nil {
		return errors.Join(errUtils.ErrStatFile, fmt.Errorf("getting info for %q: %w", srcPath, err))
	}

	if shouldSkipEntry(info, srcPath, ctx.BaseDir, ctx.Excluded, ctx.Included) {
		log.Debug("Skipping entry", "srcPath", srcPath)
		return nil
	}

	// Skip symlinks.
	if info.Mode()&os.ModeSymlink != 0 {
		log.Debug("Skipping symlink", logKeyPath, srcPath)
		return nil
	}

	if info.IsDir() {
		if err := os.MkdirAll(dstPath, info.Mode()); err != nil {
			return errors.Join(errUtils.ErrCreateDirectory, fmt.Errorf("creating directory %q: %w", dstPath, err))
		}
		// Recurse with the same context but with updated source and destination directories.
		newCtx := &CopyContext{
			SrcDir:   srcPath,
			DstDir:   dstPath,
			BaseDir:  ctx.BaseDir,
			Excluded: ctx.Excluded,
			Included: ctx.Included,
		}
		return copyDirRecursive(newCtx)
	}
	return copyFile(srcPath, dstPath)
}

// copyDirRecursive recursively copies srcDir to dstDir using shouldSkipEntry filtering.
func copyDirRecursive(ctx *CopyContext) error {
	entries, err := os.ReadDir(ctx.SrcDir)
	if err != nil {
		return errors.Join(errUtils.ErrReadDirectory, fmt.Errorf("reading directory %q: %w", ctx.SrcDir, err))
	}
	for _, entry := range entries {
		if err := processDirEntry(entry, ctx); err != nil {
			return err
		}
	}
	return nil
}

// shouldSkipPrefixEntry checks exclusion patterns for copyDirRecursiveWithPrefix.
func shouldSkipPrefixEntry(info os.FileInfo, fullRelPath string, excluded []string) bool {
	for _, pattern := range excluded {
		matched, err := u.PathMatch(pattern, fullRelPath)
		if err != nil {
			log.Debug("Error matching exclusion pattern in prefix function", logKeyPattern, pattern, logKeyPath, fullRelPath, logKeyError, err)
			continue
		}
		if matched {
			log.Debug("Excluding (prefix) due to exclusion pattern (plain match)", logKeyPath, fullRelPath, logKeyPattern, pattern)
			return true
		}
		if info.IsDir() {
			matched, err = u.PathMatch(pattern, fullRelPath+"/")
			if err != nil {
				log.Debug("Error matching exclusion pattern with trailing slash in prefix function", logKeyPattern, pattern, logKeyPath, fullRelPath+"/", logKeyError, err)
				continue
			}
			if matched {
				log.Debug("Excluding (prefix) due to exclusion pattern (with trailing slash)", logKeyPath, fullRelPath+"/", logKeyPattern, pattern)
				return true
			}
		}
	}
	return false
}

// processPrefixEntry handles a single entry for copyDirRecursiveWithPrefix.
func processPrefixEntry(entry os.DirEntry, ctx *PrefixCopyContext) error {
	fullRelPath := filepath.ToSlash(filepath.Join(ctx.Prefix, entry.Name()))
	srcPath := filepath.Join(ctx.SrcDir, entry.Name())
	dstPath := filepath.Join(ctx.DstDir, entry.Name())

	info, err := entry.Info()
	if err != nil {
		return errors.Join(errUtils.ErrStatFile, fmt.Errorf("getting info for %q: %w", srcPath, err))
	}

	if entry.Name() == ".git" {
		log.Debug("Skipping .git directory", logKeyPath, fullRelPath)
		return nil
	}

	if shouldSkipPrefixEntry(info, fullRelPath, ctx.Excluded) {
		return nil
	}

	if info.IsDir() {
		if err := os.MkdirAll(dstPath, info.Mode()); err != nil {
			return errors.Join(errUtils.ErrCreateDirectory, fmt.Errorf("creating directory %q: %w", dstPath, err))
		}
		newCtx := &PrefixCopyContext{
			SrcDir:     srcPath,
			DstDir:     dstPath,
			GlobalBase: ctx.GlobalBase,
			Prefix:     fullRelPath,
			Excluded:   ctx.Excluded,
		}
		return copyDirRecursiveWithPrefix(newCtx)
	}
	return copyFile(srcPath, dstPath)
}

// copyDirRecursiveWithPrefix recursively copies srcDir to dstDir while preserving the global relative path.
func copyDirRecursiveWithPrefix(ctx *PrefixCopyContext) error {
	entries, err := os.ReadDir(ctx.SrcDir)
	if err != nil {
		return errors.Join(errUtils.ErrReadDirectory, fmt.Errorf("reading directory %q: %w", ctx.SrcDir, err))
	}
	for _, entry := range entries {
		if err := processPrefixEntry(entry, ctx); err != nil {
			return err
		}
	}
	return nil
}

// getMatchesForPattern returns files/directories matching a pattern relative to sourceDir.
func getMatchesForPattern(sourceDir, pattern string) ([]string, error) {
	return defaultFileCopier.getMatchesForPattern(sourceDir, pattern)
}

// getMatchesForPattern returns files/directories matching a pattern relative to sourceDir.
func (fc *FileCopier) getMatchesForPattern(sourceDir, pattern string) ([]string, error) {
	fullPattern := filepath.Join(sourceDir, pattern)
	matches, err := fc.glob.GetGlobMatches(fullPattern)
	if err != nil {
		return nil, fmt.Errorf("error getting glob matches for %q: %w", fullPattern, err)
	}
	if len(matches) != 0 {
		return matches, nil
	}

	// Handle shallow copy indicator.
	if strings.HasSuffix(pattern, shallowCopySuffix) {
		if !strings.HasSuffix(pattern, "/**") {
			log.Debug("No matches found for shallow pattern; target directory will be empty", logKeyPattern, fullPattern)
			return []string{}, nil
		}
		recursivePattern := strings.TrimSuffix(pattern, shallowCopySuffix) + "/**"
		fullRecursivePattern := filepath.Join(sourceDir, recursivePattern)
		matches, err = fc.glob.GetGlobMatches(fullRecursivePattern)
		if err != nil {
			return nil, fmt.Errorf("error getting glob matches for recursive pattern %q: %w", fullRecursivePattern, err)
		}
		if len(matches) == 0 {
			log.Debug("No matches found for recursive pattern; target directory will be empty", logKeyPattern, fullRecursivePattern)
			return []string{}, nil
		}
		return matches, nil
	}

	log.Debug("No matches found for pattern; target directory will be empty", logKeyPattern, fullPattern)
	return []string{}, nil
}

// isShallowPattern determines if a pattern indicates a shallow copy.
func isShallowPattern(pattern string) bool {
	return strings.HasSuffix(pattern, shallowCopySuffix) && !strings.HasSuffix(pattern, "/**")
}

// processMatch handles a single file/directory match for CopyToTargetWithPatterns.
func processMatch(sourceDir, targetPath, file string, shallow bool, excluded []string) error {
	info, err := os.Stat(file)
	if err != nil {
		return errors.Join(errUtils.ErrStatFile, fmt.Errorf("stating file %q: %w", file, err))
	}
	relPath, err := filepath.Rel(sourceDir, file)
	if err != nil {
		return errors.Join(errUtils.ErrComputeRelativePath, fmt.Errorf("computing relative path for %q: %w", file, err))
	}
	relPath = filepath.ToSlash(relPath)
	if shouldExcludePath(info, relPath, excluded) {
		return nil
	}

	dstPath := filepath.Join(targetPath, relPath)
	if info.IsDir() {
		if shallow {
			log.Debug("Directory is not copied because it is a shallow copy", "directory", relPath)
			return nil
		}
		return copyDirRecursiveWithPrefix(&PrefixCopyContext{
			SrcDir:     file,
			DstDir:     dstPath,
			GlobalBase: sourceDir,
			Prefix:     relPath,
			Excluded:   excluded,
		})
	}
	return copyFile(file, dstPath)
}

// processIncludedPattern handles all matches for one inclusion pattern.
func processIncludedPattern(sourceDir, targetPath, pattern string, excluded []string) error {
	shallow := isShallowPattern(pattern)
	matches, err := getMatchesForPattern(sourceDir, pattern)
	if err != nil {
		log.Debug("Warning: error getting matches for pattern", logKeyPattern, pattern, logKeyError, err)
		return nil
	}
	if len(matches) == 0 {
		log.Debug("No files matched the inclusion pattern", logKeyPattern, pattern)
		return nil
	}
	for _, file := range matches {
		if err := processMatch(sourceDir, targetPath, file, shallow, excluded); err != nil {
			return err
		}
	}
	return nil
}

// initFinalTarget initializes the final target path based on source type.
func initFinalTarget(sourceDir, targetPath string, sourceIsLocalFile bool) (string, error) {
	if sourceIsLocalFile {
		return getLocalFinalTarget(sourceDir, targetPath)
	}
	return getNonLocalFinalTarget(targetPath)
}

func getLocalFinalTarget(sourceDir, targetPath string) (string, error) {
	if filepath.Ext(targetPath) == "" {
		if err := os.MkdirAll(targetPath, os.ModePerm); err != nil {
			return "", errors.Join(errUtils.ErrCreateDirectory, fmt.Errorf("creating target directory %q: %w", targetPath, err))
		}
		return filepath.Join(targetPath, SanitizeFileName(filepath.Base(sourceDir))), nil
	}

	parent := filepath.Dir(targetPath)
	if err := os.MkdirAll(parent, os.ModePerm); err != nil {
		return "", errors.Join(errUtils.ErrCreateDirectory, fmt.Errorf("creating parent directory %q: %w", parent, err))
	}
	return targetPath, nil
}

func getNonLocalFinalTarget(targetPath string) (string, error) {
	if err := os.MkdirAll(targetPath, os.ModePerm); err != nil {
		return "", errors.Join(errUtils.ErrCreateDirectory, fmt.Errorf("creating target directory %q: %w", targetPath, err))
	}
	return targetPath, nil
}

// handleLocalFileSource handles copy for local file sources.
func handleLocalFileSource(sourceDir, finalTarget string) error {
	log.Debug("Local file source detected; invoking ComponentOrMixinsCopy",
		"sourceFile", sourceDir, finalTargetKey, finalTarget)
	return ComponentOrMixinsCopy(sourceDir, finalTarget)
}

// CopyToTargetWithPatterns copies the contents from sourceDir to targetPath, applying inclusion and exclusion patterns.
func CopyToTargetWithPatterns(
	sourceDir, targetPath string,
	s *schema.AtmosVendorSource,
	sourceIsLocalFile bool,
) error {
	defer perf.Track(nil, "exec.CopyToTargetWithPatterns")()

	finalTarget, err := initFinalTarget(sourceDir, targetPath, sourceIsLocalFile)
	if err != nil {
		return err
	}

	if sourceIsLocalFile {
		return handleLocalFileSource(sourceDir, finalTarget)
	}
	// If no inclusion or exclusion patterns are defined, use the cp library.
	if len(s.IncludedPaths) == 0 && len(s.ExcludedPaths) == 0 {
		log.Debug("No inclusion or exclusion patterns defined; using cp.Copy for fast copy")
		return cp.Copy(sourceDir, finalTarget)
	}
	// Process each inclusion pattern.
	for _, pattern := range s.IncludedPaths {
		log.Debug("Processing inclusion pattern", "pattern", pattern, "source", sourceDir, finalTargetKey, finalTarget)
		if err := processIncludedPattern(sourceDir, finalTarget, pattern, s.ExcludedPaths); err != nil {
			return err
		}
	}
	// Copy entire directory if no inclusion patterns are defined.
	if len(s.IncludedPaths) == 0 {
		log.Debug("No inclusion patterns defined; copying entire directory recursively", "source", sourceDir, finalTargetKey, finalTarget)
		if err := copyDirRecursive(&CopyContext{
			SrcDir:   sourceDir,
			DstDir:   finalTarget,
			BaseDir:  sourceDir,
			Excluded: s.ExcludedPaths,
			Included: s.IncludedPaths,
		}); err != nil {
			return fmt.Errorf("error copying from %q to %q: %w", sourceDir, finalTarget, err)
		}
	}
	return nil
}

// ComponentOrMixinsCopy covers 2 cases: file-to-folder and file-to-file copy.
func ComponentOrMixinsCopy(sourceFile, finalTarget string) error {
	defer perf.Track(nil, "exec.ComponentOrMixinsCopy")()

	var dest string
	if filepath.Ext(finalTarget) == "" {
		// File-to-folder copy: append the source file's base name to the directory.
		dest = filepath.Join(finalTarget, filepath.Base(sourceFile))
		log.Debug("ComponentOrMixinsCopy: file-to-folder copy", "sourceFile", sourceFile, "destination", dest)
	} else {
		// File-to-file copy: use finalTarget as is.
		dest = finalTarget
		// Create only the parent directory.
		parent := filepath.Dir(dest)
		if err := os.MkdirAll(parent, os.ModePerm); err != nil {
			log.Debug("ComponentOrMixinsCopy: error creating parent directory", "parent", parent, "error", err)
			return errors.Join(errUtils.ErrCreateDirectory, fmt.Errorf("creating parent directory %q: %w", parent, err))
		}
		log.Debug("ComponentOrMixinsCopy: file-to-file copy", "sourceFile", sourceFile, "destination", dest)
	}
	// Remove any existing directory at dest to avoid "is a directory" errors.
	if info, err := os.Stat(dest); err == nil && info.IsDir() {
		log.Debug("ComponentOrMixinsCopy: destination exists as directory, removing", "destination", dest)
		if err := os.RemoveAll(dest); err != nil {
			return fmt.Errorf("removing existing directory %q: %w", dest, err)
		}
	}
	return cp.Copy(sourceFile, dest)
}
