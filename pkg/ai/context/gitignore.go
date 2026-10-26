package context

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// GitignoreFilter handles gitignore pattern matching.
type GitignoreFilter struct {
	basePath string
	patterns []gitignorePattern
}

// gitignorePattern represents a single gitignore pattern.
type gitignorePattern struct {
	pattern string
	negate  bool
}

// NewGitignoreFilter creates a new gitignore filter.
func NewGitignoreFilter(basePath string) (*GitignoreFilter, error) {
	filter := &GitignoreFilter{
		basePath: basePath,
		patterns: make([]gitignorePattern, 0),
	}

	// Load .gitignore from base path.
	if err := filter.loadGitignore(filepath.Join(basePath, ".gitignore")); err != nil {
		// Non-fatal: if .gitignore doesn't exist, just return empty filter.
		if !os.IsNotExist(err) {
			return nil, err
		}
	}

	return filter, nil
}

// loadGitignore loads patterns from a .gitignore file.
func (g *GitignoreFilter) loadGitignore(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for negation.
		negate := false
		if strings.HasPrefix(line, "!") {
			negate = true
			line = strings.TrimPrefix(line, "!")
		}

		g.patterns = append(g.patterns, gitignorePattern{
			pattern: line,
			negate:  negate,
		})
	}

	return scanner.Err()
}

// IsIgnored checks if a file path should be ignored.
func (g *GitignoreFilter) IsIgnored(relPath string) bool {
	// Normalize path separators.
	normalizedPath := filepath.ToSlash(relPath)

	// Track the last match result.
	ignored := false

	// Process patterns in order (later patterns override earlier ones).
	for _, p := range g.patterns {
		matched := g.matchPattern(p.pattern, normalizedPath)

		if matched {
			// If pattern is negated (!pattern), un-ignore the file.
			// If pattern is normal, ignore the file.
			ignored = !p.negate
		}
	}

	return ignored
}

// matchPattern checks if a path matches a gitignore pattern.
func (g *GitignoreFilter) matchPattern(pattern, path string) bool {
	// Handle directory-only patterns (ending with /).
	if strings.HasSuffix(pattern, "/") {
		pattern = strings.TrimSuffix(pattern, "/")
		// Check if any directory component in the path matches.
		parts := strings.Split(path, "/")
		for i := 0; i < len(parts)-1; i++ { // Exclude the file itself.
			if matched, _ := doublestar.Match(pattern, parts[i]); matched {
				return true
			}
		}
		return false
	}

	// If pattern starts with /, it's anchored to the repository root.
	if strings.HasPrefix(pattern, "/") {
		pattern = strings.TrimPrefix(pattern, "/")
		matched, _ := doublestar.Match(pattern, path)
		return matched
	}

	// If pattern contains /, it's a relative path from root.
	if strings.Contains(pattern, "/") {
		matched, _ := doublestar.Match(pattern, path)
		return matched
	}

	// Otherwise, match against any path component (basename or directory).
	// Try matching the full path.
	matched, _ := doublestar.Match(pattern, path)
	if matched {
		return true
	}

	// Try matching just the basename.
	basename := filepath.Base(path)
	matched, _ = doublestar.Match(pattern, basename)
	if matched {
		return true
	}

	// Try matching as a directory component in the path.
	// For example, "node_modules" should match "node_modules/package.json".
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if matched, _ := doublestar.Match(pattern, part); matched {
			return true
		}
	}

	return false
}
