package exec

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	l "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	cp "github.com/otiai10/copy" // Using the optimized copy library when no filtering is required.
)

// copyFile copies a single file from src to dst while preserving file permissions.
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening source file %q: %w", src, err)
	}
	defer sourceFile.Close()

	if err := os.MkdirAll(filepath.Dir(dst), os.ModePerm); err != nil {
		return fmt.Errorf("creating destination directory for %q: %w", dst, err)
	}

	destinationFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("creating destination file %q: %w", dst, err)
	}
	defer destinationFile.Close()

	if _, err := io.Copy(destinationFile, sourceFile); err != nil {
		return fmt.Errorf("copying content from %q to %q: %w", src, dst, err)
	}

	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("getting file info for %q: %w", src, err)
	}
	if err := os.Chmod(dst, info.Mode()); err != nil {
		return fmt.Errorf("setting permissions on %q: %w", dst, err)
	}
	return nil
}

// shouldSkipEntry determines whether to skip a file/directory based on its relative path to baseDir.
// If an error occurs during matching for an exclusion or inclusion pattern, it logs the error and proceeds.
func shouldSkipEntry(info os.FileInfo, srcPath, baseDir string, excluded, included []string) (bool, error) {
	if info.Name() == ".git" {
		return true, nil
	}
	relPath, err := filepath.Rel(baseDir, srcPath)
	if err != nil {
		l.Debug("Error computing relative path", "srcPath", srcPath, "error", err)
		return true, nil // treat error as a signal to skip
	}
	// Ensure uniform path separator.
	relPath = filepath.ToSlash(relPath)

	// Process exclusion patterns.
	// For directories, check with and without a trailing slash.
	for _, pattern := range excluded {
		// First check the plain relative path.
		matched, err := u.PathMatch(pattern, relPath)
		if err != nil {
			l.Debug("Error matching exclusion pattern", "pattern", pattern, "path", relPath, "error", err)
			continue
		}
		if matched {
			l.Debug("Excluding path due to exclusion pattern (plain match)", "path", relPath, "pattern", pattern)
			return true, nil
		}
		// If it is a directory, also try matching with a trailing slash.
		if info.IsDir() {
			matched, err = u.PathMatch(pattern, relPath+"/")
			if err != nil {
				l.Debug("Error matching exclusion pattern with trailing slash", "pattern", pattern, "path", relPath+"/", "error", err)
				continue
			}
			if matched {
				l.Debug("Excluding directory due to exclusion pattern (with trailing slash)", "path", relPath+"/", "pattern", pattern)
				return true, nil
			}
		}
	}

	// Process inclusion patterns (only for non-directory files).
	// (Directories are generally picked up by the inclusion branch in copyToTargetWithPatterns.)
	if len(included) > 0 && !info.IsDir() {
		matchedAny := false
		for _, pattern := range included {
			matched, err := u.PathMatch(pattern, relPath)
			if err != nil {
				l.Debug("Error matching inclusion pattern", "pattern", pattern, "path", relPath, "error", err)
				continue
			}
			if matched {
				l.Debug("Including path due to inclusion pattern", "path", relPath, "pattern", pattern)
				matchedAny = true
				break
			}
		}
		if !matchedAny {
			l.Debug("Excluding path because it does not match any inclusion pattern", "path", relPath)
			return true, nil
		}
	}
	return false, nil
}

// copyDirRecursive recursively copies srcDir to dstDir using shouldSkipEntry filtering.
// This function is used in cases where the entire sourceDir is the base for relative paths.
func copyDirRecursive(srcDir, dstDir, baseDir string, excluded, included []string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("reading directory %q: %w", srcDir, err)
	}
	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())

		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("getting info for %q: %w", srcPath, err)
		}

		// Check if this entry should be skipped.
		skip, err := shouldSkipEntry(info, srcPath, baseDir, excluded, included)
		if err != nil {
			return err
		}
		if skip {
			l.Debug("Skipping entry", "srcPath", srcPath)
			continue
		}

		// Skip symlinks.
		if info.Mode()&os.ModeSymlink != 0 {
			l.Debug("Skipping symlink", "path", srcPath)
			continue
		}

		if info.IsDir() {
			if err := os.MkdirAll(dstPath, info.Mode()); err != nil {
				return fmt.Errorf("creating directory %q: %w", dstPath, err)
			}
			if err := copyDirRecursive(srcPath, dstPath, baseDir, excluded, included); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// copyDirRecursiveWithPrefix recursively copies srcDir to dstDir while preserving the global relative path.
// Instead of using the local srcDir as the base for computing relative paths, this function uses the original
// source directory (globalBase) and an accumulated prefix that represents the relative path from globalBase.
func copyDirRecursiveWithPrefix(srcDir, dstDir, globalBase, prefix string, excluded []string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("reading directory %q: %w", srcDir, err)
	}
	for _, entry := range entries {
		// Compute the full relative path from the original source.
		fullRelPath := filepath.ToSlash(filepath.Join(prefix, entry.Name()))
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())

		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("getting info for %q: %w", srcPath, err)
		}

		// Skip .git directories.
		if entry.Name() == ".git" {
			l.Debug("Skipping .git directory", "path", fullRelPath)
			continue
		}

		// Check exclusion patterns using the full relative path.
		skip := false
		for _, pattern := range excluded {
			// Check plain match.
			matched, err := u.PathMatch(pattern, fullRelPath)
			if err != nil {
				l.Debug("Error matching exclusion pattern in prefix function", "pattern", pattern, "path", fullRelPath, "error", err)
				continue
			}
			if matched {
				l.Debug("Excluding (prefix) due to exclusion pattern (plain match)", "path", fullRelPath, "pattern", pattern)
				skip = true
				break
			}
			// For directories, also try with a trailing slash.
			if info.IsDir() {
				matched, err = u.PathMatch(pattern, fullRelPath+"/")
				if err != nil {
					l.Debug("Error matching exclusion pattern with trailing slash in prefix function", "pattern", pattern, "path", fullRelPath+"/", "error", err)
					continue
				}
				if matched {
					l.Debug("Excluding (prefix) due to exclusion pattern (with trailing slash)", "path", fullRelPath+"/", "pattern", pattern)
					skip = true
					break
				}
			}
		}
		if skip {
			continue
		}

		if info.IsDir() {
			if err := os.MkdirAll(dstPath, info.Mode()); err != nil {
				return fmt.Errorf("creating directory %q: %w", dstPath, err)
			}
			// Recurse with updated prefix.
			if err := copyDirRecursiveWithPrefix(srcPath, dstPath, globalBase, fullRelPath, excluded); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// getMatchesForPattern returns files/directories matching a pattern relative to sourceDir.
// If no matches are found, it logs a debug message and returns an empty slice.
// For patterns ending with "/*" (shallow copy indicator) the function does not fallback to a recursive variant.
func getMatchesForPattern(sourceDir, pattern string) ([]string, error) {
	fullPattern := filepath.Join(sourceDir, pattern)
	matches, err := u.GetGlobMatches(fullPattern)
	if err != nil {
		return nil, fmt.Errorf("error getting glob matches for %q: %w", fullPattern, err)
	}
	if len(matches) == 0 {
		// If the pattern ends with "/*" (and not "/**"), do not fallback.
		if strings.HasSuffix(pattern, "/*") && !strings.HasSuffix(pattern, "/**") {
			l.Debug("No matches found for shallow pattern; target directory will be empty", "pattern", fullPattern)
			return []string{}, nil
		}
		// Fallback for patterns ending with "/*" (non-shallow) or others.
		if strings.HasSuffix(pattern, "/*") {
			recursivePattern := strings.TrimSuffix(pattern, "/*") + "/**"
			fullRecursivePattern := filepath.Join(sourceDir, recursivePattern)
			matches, err = u.GetGlobMatches(fullRecursivePattern)
			if err != nil {
				return nil, fmt.Errorf("error getting glob matches for recursive pattern %q: %w", fullRecursivePattern, err)
			}
			if len(matches) == 0 {
				l.Debug("No matches found for recursive pattern; target directory will be empty", "pattern", fullRecursivePattern)
				return []string{}, nil
			}
			return matches, nil
		}
		l.Debug("No matches found for pattern; target directory will be empty", "pattern", fullPattern)
		return []string{}, nil
	}
	return matches, nil
}

// copyToTargetWithPatterns copies the contents from sourceDir to targetPath,
// applying inclusion and exclusion patterns from the vendor source configuration.
// If sourceIsLocalFile is true and targetPath lacks an extension, the sanitized URI is appended.
// If no included paths are defined, all files (except those matching excluded paths) are copied.
// In the special case where neither inclusion nor exclusion patterns are defined,
// the optimized cp library (github.com/otiai10/copy) is used.
func copyToTargetWithPatterns(
	sourceDir, targetPath string,
	s *schema.AtmosVendorSource,
	sourceIsLocalFile bool,
	uri string,
) error {
	if sourceIsLocalFile && filepath.Ext(targetPath) == "" {
		targetPath = filepath.Join(targetPath, SanitizeFileName(uri))
	}
	l.Debug("Copying files", "source", sourceDir, "target", targetPath)
	if err := os.MkdirAll(targetPath, os.ModePerm); err != nil {
		return fmt.Errorf("creating target directory %q: %w", targetPath, err)
	}

	// Optimization: if no inclusion and no exclusion patterns are defined, use the cp library for fast copying.
	if len(s.IncludedPaths) == 0 && len(s.ExcludedPaths) == 0 {
		l.Debug("No inclusion or exclusion patterns defined; using cp library for fast copy")
		return cp.Copy(sourceDir, targetPath)
	}

	// If inclusion patterns are provided, process each pattern individually.
	for _, pattern := range s.IncludedPaths {
		// Determine if the pattern indicates shallow copy.
		shallow := false
		if strings.HasSuffix(pattern, "/*") && !strings.HasSuffix(pattern, "/**") {
			shallow = true
		}

		matches, err := getMatchesForPattern(sourceDir, pattern)
		if err != nil {
			l.Debug("Warning: error getting matches for pattern", "pattern", pattern, "error", err)
			continue
		}
		if len(matches) == 0 {
			l.Debug("No files matched the inclusion pattern", "pattern", pattern)
			continue
		}
		for _, file := range matches {
			// Retrieve file information.
			info, err := os.Stat(file)
			if err != nil {
				return fmt.Errorf("stating file %q: %w", file, err)
			}
			relPath, err := filepath.Rel(sourceDir, file)
			if err != nil {
				return fmt.Errorf("computing relative path for %q: %w", file, err)
			}
			relPath = filepath.ToSlash(relPath)

			// Check exclusion patterns (for directories, try both plain and trailing slash).
			skip := false
			for _, ex := range s.ExcludedPaths {
				if info.IsDir() {
					matched, err := u.PathMatch(ex, relPath)
					if err != nil {
						l.Debug("Error matching exclusion pattern", "pattern", ex, "path", relPath, "error", err)
					} else if matched {
						l.Debug("Excluding directory due to exclusion pattern (plain match)", "directory", relPath, "pattern", ex)
						skip = true
						break
					}
					matched, err = u.PathMatch(ex, relPath+"/")
					if err != nil {
						l.Debug("Error matching exclusion pattern with trailing slash", "pattern", ex, "path", relPath+"/", "error", err)
					} else if matched {
						l.Debug("Excluding directory due to exclusion pattern (with trailing slash)", "directory", relPath+"/", "pattern", ex)
						skip = true
						break
					}
				} else {
					matched, err := u.PathMatch(ex, relPath)
					if err != nil {
						l.Debug("Error matching exclusion pattern", "pattern", ex, "path", relPath, "error", err)
					} else if matched {
						l.Debug("Excluding file due to exclusion pattern", "file", relPath, "pattern", ex)
						skip = true
						break
					}
				}
			}
			if skip {
				continue
			}

			// Build the destination path.
			dstPath := filepath.Join(targetPath, relPath)
			if info.IsDir() {
				if shallow {
					// Use shallow copy: copy only immediate file entries.
					l.Debug("Directory is not copied becasue it is a shallow copy", "directory", relPath)
				} else {
					// Use the existing recursive copy with prefix.
					if err := copyDirRecursiveWithPrefix(file, dstPath, sourceDir, relPath, s.ExcludedPaths); err != nil {
						return err
					}
				}
			} else {
				if err := copyFile(file, dstPath); err != nil {
					return err
				}
			}
		}
	}

	// If no inclusion patterns are defined; copy everything except those matching excluded items.
	// (This branch is preserved from the original logic.)
	if len(s.IncludedPaths) == 0 {
		if err := copyDirRecursive(sourceDir, targetPath, sourceDir, s.ExcludedPaths, s.IncludedPaths); err != nil {
			return fmt.Errorf("error copying from %q to %q: %w", sourceDir, targetPath, err)
		}
	}
	return nil
}
