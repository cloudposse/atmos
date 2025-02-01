package exec

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	cp "github.com/otiai10/copy" // Using the optimized copy library when no filtering is required.
)

// copyFile copies a single file from src to dst while preserving file permissions.
func copyFile(atmosConfig schema.AtmosConfiguration, src, dst string) error {
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

// skipFunc determines whether to skip a file/directory based on its relative path to baseDir.
// If an error occurs during matching for an exclusion or inclusion pattern, it logs the error and proceeds.
func skipFunc(atmosConfig schema.AtmosConfiguration, info os.FileInfo, srcPath, baseDir string, excluded, included []string) (bool, error) {
	if info.Name() == ".git" {
		return true, nil
	}
	relPath, err := filepath.Rel(baseDir, srcPath)
	if err != nil {
		u.LogTrace(atmosConfig, fmt.Sprintf("Error computing relative path for %q: %v", srcPath, err))
		return true, nil // treat error as a signal to skip
	}
	relPath = filepath.ToSlash(relPath)

	// Process exclusion patterns.
	for _, pattern := range excluded {
		matched, err := u.PathMatch(pattern, relPath)
		if err != nil {
			u.LogTrace(atmosConfig, fmt.Sprintf("Error matching exclusion pattern %q with %q: %v", pattern, relPath, err))
			continue
		} else if matched {
			u.LogTrace(atmosConfig, fmt.Sprintf("Excluding %q because it matches exclusion pattern %q", relPath, pattern))
			return true, nil
		}
	}

	// Process inclusion patterns (only for non-directory files).
	if len(included) > 0 && !info.IsDir() {
		matchedAny := false
		for _, pattern := range included {
			matched, err := u.PathMatch(pattern, relPath)
			if err != nil {
				u.LogTrace(atmosConfig, fmt.Sprintf("Error matching inclusion pattern %q with %q: %v", pattern, relPath, err))
				continue
			} else if matched {
				u.LogTrace(atmosConfig, fmt.Sprintf("Including %q because it matches inclusion pattern %q", relPath, pattern))
				matchedAny = true
				break
			}
		}
		if !matchedAny {
			u.LogTrace(atmosConfig, fmt.Sprintf("Excluding %q because it does not match any inclusion pattern", relPath))
			return true, nil
		}
	}
	return false, nil
}

// copyDirRecursive recursively copies srcDir to dstDir using skipFunc filtering.
func copyDirRecursive(atmosConfig schema.AtmosConfiguration, srcDir, dstDir, baseDir string, excluded, included []string) error {
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

		skip, err := skipFunc(atmosConfig, info, srcPath, baseDir, excluded, included)
		if err != nil {
			return err
		}
		if skip {
			continue
		}

		// Skip symlinks.
		if info.Mode()&os.ModeSymlink != 0 {
			u.LogTrace(atmosConfig, fmt.Sprintf("Skipping symlink: %q", srcPath))
			continue
		}

		if info.IsDir() {
			if err := os.MkdirAll(dstPath, info.Mode()); err != nil {
				return fmt.Errorf("creating directory %q: %w", dstPath, err)
			}
			if err := copyDirRecursive(atmosConfig, srcPath, dstPath, baseDir, excluded, included); err != nil {
				return err
			}
		} else {
			if err := copyFile(atmosConfig, srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// getMatchesForPattern returns files/directories matching a pattern relative to sourceDir.
// If no matches are found, it logs a trace and returns an empty slice.
// When the pattern ends with "/*", it retries with a recursive "/**" variant.
func getMatchesForPattern(atmosConfig schema.AtmosConfiguration, sourceDir, pattern string) ([]string, error) {
	fullPattern := filepath.Join(sourceDir, pattern)
	matches, err := u.GetGlobMatches(fullPattern)
	if err != nil {
		return nil, fmt.Errorf("error getting glob matches for %q: %w", fullPattern, err)
	}
	if len(matches) == 0 {
		if strings.HasSuffix(pattern, "/*") {
			recursivePattern := strings.TrimSuffix(pattern, "/*") + "/**"
			fullRecursivePattern := filepath.Join(sourceDir, recursivePattern)
			matches, err = u.GetGlobMatches(fullRecursivePattern)
			if err != nil {
				return nil, fmt.Errorf("error getting glob matches for recursive pattern %q: %w", fullRecursivePattern, err)
			}
			if len(matches) == 0 {
				u.LogTrace(atmosConfig, fmt.Sprintf("No matches found for recursive pattern %q - target directory will be empty", fullRecursivePattern))
				return []string{}, nil
			}
			return matches, nil
		}
		u.LogTrace(atmosConfig, fmt.Sprintf("No matches found for pattern %q - target directory will be empty", fullPattern))
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
	atmosConfig schema.AtmosConfiguration,
	sourceDir, targetPath string,
	s *schema.AtmosVendorSource,
	sourceIsLocalFile bool,
	uri string,
) error {
	if sourceIsLocalFile && filepath.Ext(targetPath) == "" {
		targetPath = filepath.Join(targetPath, SanitizeFileName(uri))
	}
	u.LogTrace(atmosConfig, fmt.Sprintf("Copying from %q to %q", sourceDir, targetPath))
	if err := os.MkdirAll(targetPath, os.ModePerm); err != nil {
		return fmt.Errorf("creating target directory %q: %w", targetPath, err)
	}

	// Optimization: if no inclusion and no exclusion patterns are defined, use the cp library for fast copying.
	if len(s.IncludedPaths) == 0 && len(s.ExcludedPaths) == 0 {
		u.LogTrace(atmosConfig, "No inclusion or exclusion patterns defined; using cp library for fast copy")
		return cp.Copy(sourceDir, targetPath)
	}

	// If inclusion patterns are provided, use them to determine which files to copy.
	if len(s.IncludedPaths) > 0 {
		filesToCopy := make(map[string]struct{})
		for _, pattern := range s.IncludedPaths {
			matches, err := getMatchesForPattern(atmosConfig, sourceDir, pattern)
			if err != nil {
				u.LogTrace(atmosConfig, fmt.Sprintf("Warning: error getting matches for pattern %q: %v", pattern, err))
				continue
			}
			for _, match := range matches {
				filesToCopy[match] = struct{}{}
			}
		}
		if len(filesToCopy) == 0 {
			u.LogTrace(atmosConfig, "No files matched the inclusion patterns - target directory will be empty")
			return nil
		}
		for file := range filesToCopy {
			relPath, err := filepath.Rel(sourceDir, file)
			if err != nil {
				return fmt.Errorf("computing relative path for %q: %w", file, err)
			}
			relPath = filepath.ToSlash(relPath)
			skip := false
			for _, ex := range s.ExcludedPaths {
				matched, err := u.PathMatch(ex, relPath)
				if err != nil {
					u.LogTrace(atmosConfig, fmt.Sprintf("Error matching exclusion pattern %q with %q: %v", ex, relPath, err))
					continue
				} else if matched {
					u.LogTrace(atmosConfig, fmt.Sprintf("Excluding %q because it matches exclusion pattern %q", relPath, ex))
					skip = true
					break
				}
			}
			if skip {
				continue
			}
			dstPath := filepath.Join(targetPath, relPath)
			info, err := os.Stat(file)
			if err != nil {
				return fmt.Errorf("stating file %q: %w", file, err)
			}
			if info.IsDir() {
				if err := copyDirRecursive(atmosConfig, file, dstPath, file, s.ExcludedPaths, nil); err != nil {
					return err
				}
			} else {
				if err := copyFile(atmosConfig, file, dstPath); err != nil {
					return err
				}
			}
		}
	} else {
		// No inclusion patterns defined; copy everything except those matching excluded items.
		if err := copyDirRecursive(atmosConfig, sourceDir, targetPath, sourceDir, s.ExcludedPaths, s.IncludedPaths); err != nil {
			return fmt.Errorf("error copying from %q to %q: %w", sourceDir, targetPath, err)
		}
	}
	return nil
}
