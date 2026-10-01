package filematch

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cloudposse/atmos/pkg/filesystem"
)

// globMetaCharacters are the characters that make a pattern segment a glob
// expression rather than a literal path component.
const globMetaCharacters = "*?[]{}!"

// matcher is the main struct for matching files with injected dependencies.
type matcher struct {
	fs    filesystem.FileSystem
	globC globCompiler
}

// MatchFiles takes a slice of gitignore-style patterns and returns matching file paths.
func (m *matcher) MatchFiles(patterns []string) ([]string, error) {
	var matches []string
	cwd, err := m.fs.Getwd()
	if err != nil {
		return nil, err
	}

	for _, pattern := range patterns {
		basePath, globPattern := extractBasePathAndGlob(pattern)
		if !filepath.IsAbs(basePath) {
			basePath = filepath.Join(cwd, basePath)
		}

		// A literal pattern (no glob metacharacters) matches at most one path:
		// check it directly instead of walking the whole tree. Without this, a
		// plain file name like "atmos.yaml" walks every file under basePath
		// (including .git and node_modules), which dominated
		// `atmos validate schema` latency in large repositories.
		if !strings.ContainsAny(globPattern, globMetaCharacters) {
			candidate := filepath.Join(basePath, filepath.FromSlash(globPattern))
			if info, err := m.fs.Stat(candidate); err == nil && !info.IsDir() {
				matches = append(matches, candidate)
			}
			continue
		}

		isRecursive := strings.Contains(globPattern, "**")

		if isRecursive {
			globPattern = strings.ReplaceAll(globPattern, "*/*", "")
		}

		g, err := m.globC.Compile(globPattern)
		if err != nil {
			return nil, ErrInvalidPattern{Pattern: pattern, Err: err}
		}

		err = m.fs.Walk(basePath, m.createWalkFunc(basePath, g, isRecursive, &matches))
		if err != nil {
			return nil, err
		}
	}

	return matches, nil
}

func (m *matcher) createWalkFunc(basePath string, g compiledGlob, isRecursive bool, matches *[]string) filepath.WalkFunc {
	return func(path string, info os.FileInfo, filePathErr error) error {
		if filePathErr != nil {
			return filePathErr
		}
		if info.IsDir() {
			return nil
		}
		relPath, err := filepath.Rel(basePath, path)
		if err != nil {
			return err
		}
		depth := strings.Count(relPath, string(filepath.Separator))
		if !isRecursive && depth > 0 {
			return nil
		}
		if g.Match(filepath.ToSlash(relPath)) {
			*matches = append(*matches, path)
		}
		return nil
	}
}

// extractBasePathAndGlob extracts the base path and the glob pattern from the input pattern.
func extractBasePathAndGlob(pattern string) (string, string) {
	// Check if the pattern starts with a slash
	hasLeadingSlash := strings.HasPrefix(pattern, "/")

	// Split the pattern into parts
	parts := strings.Split(pattern, string(filepath.Separator))

	// Find the index where the glob pattern starts
	var globStartIndex int
	for i, part := range parts {
		if strings.ContainsAny(part, globMetaCharacters) {
			globStartIndex = i
			break
		}
	}
	if runtime.GOOS == "windows" && len(parts) > 0 && strings.HasSuffix(parts[0], ":") {
		parts[0] += string(os.PathSeparator)
	}
	// Extract the base path (everything before the glob starts)
	basePath := filepath.Join(parts[:globStartIndex]...)

	// If the original pattern had a leading slash, prepend it to the base path
	if hasLeadingSlash {
		basePath = "/" + basePath
	}

	// Extract the glob pattern (everything after the base path)
	globPattern := strings.Join(parts[globStartIndex:], "/")

	// Handle trailing slashes (e.g., "dir/" -> "dir/**")
	if strings.HasSuffix(pattern, "/") {
		globPattern = filepath.Join(globPattern, "**")
	}

	return basePath, globPattern
}

// Convenience function for default usage.
func NewGlobMatcher() *matcher {
	fs := filesystem.NewOSFileSystem()
	globC := NewDefaultGlobCompiler()
	return &matcher{
		fs:    fs,
		globC: globC,
	}
}
