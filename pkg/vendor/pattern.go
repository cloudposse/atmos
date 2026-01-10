package vendor

import (
	"path/filepath"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// Pattern matching constants.
const (
	patternDoublestar = "**"
	patternSinglestar = "*"

	// Log field keys.
	logKeyIncludedPaths = "included_paths"
	logKeyPath          = "path"
)

// shouldSkipBasedOnIncludedPaths checks if a file/directory should be skipped based on included_paths patterns.
// For files: returns true (skip) if the file doesn't match any included pattern.
// For directories: returns false (don't skip) if any pattern COULD match files under this directory.
func shouldSkipBasedOnIncludedPaths(isDir bool, fullPath, relativePath string, includedPaths []string) (bool, error) {
	if !isDir {
		return shouldSkipFile(fullPath, relativePath, includedPaths)
	}
	return shouldSkipDirectory(fullPath, relativePath, includedPaths)
}

// shouldSkipFile checks if a file should be skipped based on included_paths patterns.
func shouldSkipFile(fullPath, relativePath string, includedPaths []string) (bool, error) {
	for _, includePath := range includedPaths {
		matched, err := matchFileAgainstPattern(includePath, fullPath, relativePath)
		if err != nil {
			return true, err
		}
		if matched {
			log.Debug("Including file since it matches pattern from 'included_paths'", logKeyIncludedPaths, includePath, logKeyPath, relativePath)
			return false, nil
		}
	}
	log.Debug("Excluding file since it does not match any pattern from 'included_paths'", logKeyPath, relativePath)
	return true, nil
}

// matchFileAgainstPattern checks if a file matches a pattern using both full and relative paths.
func matchFileAgainstPattern(pattern, fullPath, relativePath string) (bool, error) {
	// Try matching with full path (for patterns like "**/foo.md").
	if match, err := u.PathMatch(pattern, fullPath); err != nil {
		return false, err
	} else if match {
		return true, nil
	}

	// Try matching with relative path, handling leading '/'.
	pathToMatch := relativePath
	if strings.HasPrefix(pattern, "/") {
		pathToMatch = "/" + relativePath
	}
	return u.PathMatch(pattern, pathToMatch)
}

// shouldSkipDirectory checks if a directory should be skipped based on included_paths patterns.
func shouldSkipDirectory(fullPath, relativePath string, includedPaths []string) (bool, error) {
	for _, includePath := range includedPaths {
		shouldInclude, err := checkDirectoryAgainstPattern(includePath, fullPath, relativePath)
		if err != nil {
			return true, err
		}
		if shouldInclude {
			return false, nil
		}
	}
	log.Debug("Excluding directory since no pattern could match files under it", logKeyPath, relativePath)
	return true, nil
}

// checkDirectoryAgainstPattern checks if a directory should be included for a single pattern.
func checkDirectoryAgainstPattern(pattern, fullPath, relativePath string) (bool, error) {
	// Check if directory itself matches the pattern.
	if match, err := matchDirectoryDirectly(pattern, fullPath, relativePath); err != nil {
		return false, err
	} else if match {
		log.Debug("Including directory since it matches pattern from 'included_paths'", logKeyIncludedPaths, pattern, logKeyPath, relativePath)
		return true, nil
	}

	// Check if pattern could match direct children of this directory.
	if match, err := matchDirectoryChildren(pattern, fullPath, relativePath); err != nil {
		return false, err
	} else if match {
		log.Debug("Including directory since pattern could match direct children", logKeyIncludedPaths, pattern, logKeyPath, relativePath)
		return true, nil
	}

	// Check if pattern could match nested files under this directory.
	if couldMatchNestedPath(relativePath, pattern) {
		log.Debug("Including directory since pattern could match nested files", logKeyIncludedPaths, pattern, logKeyPath, relativePath)
		return true, nil
	}

	return false, nil
}

// matchDirectoryDirectly checks if a directory directly matches a pattern.
func matchDirectoryDirectly(pattern, fullPath, relativePath string) (bool, error) {
	if match, err := u.PathMatch(pattern, fullPath); err != nil {
		return false, err
	} else if match {
		return true, nil
	}

	// For patterns with leading '/', also check against relative path with leading '/'.
	if strings.HasPrefix(pattern, "/") {
		return u.PathMatch(pattern, "/"+relativePath)
	}
	return false, nil
}

// matchDirectoryChildren checks if a pattern could match direct children of a directory.
func matchDirectoryChildren(pattern, fullPath, relativePath string) (bool, error) {
	if match, err := u.PathMatch(pattern, fullPath+"/*"); err != nil {
		return false, err
	} else if match {
		return true, nil
	}

	// For patterns with leading '/', also try with relative path.
	if strings.HasPrefix(pattern, "/") {
		return u.PathMatch(pattern, "/"+relativePath+"/*")
	}
	return false, nil
}

// couldMatchNestedPath checks if a directory could be part of a path that matches the pattern.
// For example, directory "examples" could be part of path "examples/demo-library/foo.md"
// which matches pattern "**/demo-library/**/*.md".
func couldMatchNestedPath(dirPath, pattern string) bool {
	// Normalize paths.
	dirPath = filepath.ToSlash(dirPath)
	pattern = filepath.ToSlash(pattern)

	// Check patterns starting with ** first (most permissive).
	if strings.HasPrefix(pattern, "**/") || strings.HasPrefix(pattern, "**/{") {
		return checkDoublestarPrefixPattern(dirPath, pattern)
	}

	// Check patterns containing ** elsewhere.
	if strings.Contains(pattern, patternDoublestar) {
		return checkDoublestarPattern(dirPath, pattern)
	}

	// For patterns without **, check if directory path is a prefix of the pattern.
	return checkSimplePattern(dirPath, pattern)
}

// checkDoublestarPrefixPattern handles patterns that start with **.
func checkDoublestarPrefixPattern(dirPath, pattern string) bool {
	dirParts := strings.Split(strings.TrimPrefix(dirPath, "/"), "/")
	requiredDirs := extractRequiredDirs(pattern)

	if len(requiredDirs) == 0 {
		return false
	}

	// If any directory segment matches a required directory, include it.
	if containsAny(dirParts, requiredDirs) {
		return true
	}

	// We must traverse ALL directories to find required ones.
	// Files will be filtered later, so empty directories might be created but that's OK.
	return true
}

// checkDoublestarPattern handles patterns containing ** (not at start).
func checkDoublestarPattern(dirPath, pattern string) bool {
	dirBase := filepath.Base(dirPath)
	patternParts := strings.Split(strings.TrimPrefix(pattern, "/"), "/")

	// Check if directory name matches any concrete part of the pattern.
	if matchesPatternPart(dirBase, patternParts) {
		return true
	}

	// Check if directory path is a prefix of the non-** part of the pattern.
	return checkPatternPrefix(dirPath, pattern)
}

// checkSimplePattern handles patterns without **.
func checkSimplePattern(dirPath, pattern string) bool {
	patternPath := strings.TrimPrefix(pattern, "/")
	dirPathClean := strings.TrimPrefix(dirPath, "/")
	return strings.HasPrefix(patternPath, dirPathClean+"/") ||
		strings.HasPrefix(patternPath, dirPathClean+"/*")
}

// containsAny checks if any element in parts matches any element in targets.
func containsAny(parts, targets []string) bool {
	for _, part := range parts {
		for _, target := range targets {
			if part == target {
				return true
			}
		}
	}
	return false
}

// matchesPatternPart checks if a directory name matches any concrete part of a pattern.
func matchesPatternPart(dirBase string, patternParts []string) bool {
	for _, part := range patternParts {
		if part == patternDoublestar || part == patternSinglestar {
			continue
		}
		if matchesBraceOrConcrete(dirBase, part) {
			return true
		}
	}
	return false
}

// matchesBraceOrConcrete checks if dirBase matches a brace pattern or concrete name.
func matchesBraceOrConcrete(dirBase, part string) bool {
	if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
		options := strings.Split(strings.Trim(part, "{}"), ",")
		for _, opt := range options {
			if dirBase == opt {
				return true
			}
		}
		return false
	}
	if !strings.ContainsAny(part, "*?{}[]") {
		return dirBase == part
	}
	return false
}

// checkPatternPrefix checks if directory is a prefix of pattern's non-** prefix.
func checkPatternPrefix(dirPath, pattern string) bool {
	nonWildcardPrefix := getPatternPrefixBeforeDoublestar(pattern)
	if nonWildcardPrefix == "" {
		return false
	}
	dirPathClean := strings.TrimPrefix(dirPath, "/")
	return strings.HasPrefix(nonWildcardPrefix, dirPathClean+"/") ||
		strings.HasPrefix(dirPathClean, nonWildcardPrefix) ||
		nonWildcardPrefix == dirPathClean
}

// getPatternPrefixBeforeDoublestar returns the concrete path prefix before the first "**".
// For example, "foo/bar/**/baz/*.md" returns "foo/bar".
func getPatternPrefixBeforeDoublestar(pattern string) string {
	parts := strings.Split(filepath.ToSlash(pattern), "/")
	var prefix []string
	for _, part := range parts {
		if part == patternDoublestar || strings.ContainsAny(part, "*?{}[]") {
			break
		}
		prefix = append(prefix, part)
	}
	return strings.Join(prefix, "/")
}

// extractRequiredDirs extracts concrete directory names required by a pattern.
// For patterns like "**/{demo-library,demo-stacks}/**/*.md", it returns ["demo-library", "demo-stacks"].
func extractRequiredDirs(pattern string) []string {
	var dirs []string
	parts := strings.Split(strings.TrimPrefix(filepath.ToSlash(pattern), "/"), "/")

	for _, part := range parts {
		dirs = append(dirs, extractDirsFromPart(part)...)
	}

	return dirs
}

// extractDirsFromPart extracts directory names from a single pattern part.
func extractDirsFromPart(part string) []string {
	if part == patternDoublestar || part == patternSinglestar || part == "" {
		return nil
	}
	// Handle brace expansion.
	if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
		return extractDirsFromBraceExpansion(part)
	}
	// Handle concrete directory or file name.
	if !strings.ContainsAny(part, "*?{}[]") {
		return extractConcreteDirName(part)
	}
	return nil
}

// extractDirsFromBraceExpansion extracts directory names from a brace expansion like "{foo,bar}".
func extractDirsFromBraceExpansion(part string) []string {
	var dirs []string
	options := strings.Split(strings.Trim(part, "{}"), ",")
	for _, opt := range options {
		// Only add if it's a concrete name (no wildcards).
		if !strings.ContainsAny(opt, "*?[]") {
			dirs = append(dirs, opt)
		}
	}
	return dirs
}

// extractConcreteDirName returns the part as a directory name if it doesn't look like a file.
func extractConcreteDirName(part string) []string {
	// Skip file extensions for directory matching.
	if !strings.Contains(part, ".") || strings.HasSuffix(part, "/") {
		return []string{part}
	}
	return nil
}
