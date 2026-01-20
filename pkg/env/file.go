// Package env provides utilities for working with environment variables.
package env

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/joho/godotenv"

	"github.com/cloudposse/atmos/pkg/perf"
)

// LoadEnvFiles loads .env files matching the given patterns from basePath.
// Patterns support globs (e.g., ".env.*").
// Missing files are silently ignored.
// Returns merged env vars and list of loaded files (in load order).
func LoadEnvFiles(basePath string, patterns []string) (map[string]string, []string, error) {
	defer perf.Track(nil, "env.LoadEnvFiles")()

	if len(patterns) == 0 {
		return map[string]string{}, nil, nil
	}

	return loadEnvFilesFromDir(basePath, patterns)
}

// LoadFromDirectory loads .env files matching patterns from a specific directory.
// If parents is true, walks up from dir to repoRoot, loading files from each directory.
// Load order when parents=true: repo root first → intermediate dirs → working dir (closer overrides farther).
// Returns merged env vars and list of loaded files (in load order).
func LoadFromDirectory(dir string, patterns []string, parents bool, repoRoot string) (map[string]string, []string, error) {
	defer perf.Track(nil, "env.LoadFromDirectory")()

	if len(patterns) == 0 {
		return map[string]string{}, nil, nil
	}

	if !parents {
		// Simple case: just load from the specified directory.
		return loadEnvFilesFromDir(dir, patterns)
	}

	// Walk up from dir to repoRoot, collecting directories.
	dirs, err := collectParentDirs(dir, repoRoot)
	if err != nil {
		return nil, nil, err
	}

	// Load from each directory, starting from repo root (lowest priority).
	// Later directories (closer to working dir) override earlier ones.
	merged := make(map[string]string)
	var allLoadedFiles []string

	for _, d := range dirs {
		envVars, loadedFiles, err := loadEnvFilesFromDir(d, patterns)
		if err != nil {
			return nil, nil, err
		}
		// Merge: later values override earlier.
		for k, v := range envVars {
			merged[k] = v
		}
		allLoadedFiles = append(allLoadedFiles, loadedFiles...)
	}

	return merged, allLoadedFiles, nil
}

// MergeEnvMaps merges multiple env maps with later maps taking precedence.
func MergeEnvMaps(maps ...map[string]string) map[string]string {
	defer perf.Track(nil, "env.MergeEnvMaps")()

	result := make(map[string]string)
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}

// MergeEnvSlices merges env slices ([]string{"KEY=value"}) with later slices taking precedence.
func MergeEnvSlices(slices ...[]string) []string {
	defer perf.Track(nil, "env.MergeEnvSlices")()

	merged := make(map[string]string)
	for _, slice := range slices {
		for _, entry := range slice {
			if idx := strings.Index(entry, "="); idx > 0 {
				key := entry[:idx]
				value := entry[idx+1:]
				merged[key] = value
			}
		}
	}

	return ConvertMapToSlice(merged)
}

// MapToSlice converts a map[string]string to []string{"KEY=value"}.
// This is an alias for ConvertMapToSlice for readability in certain contexts.
func MapToSlice(m map[string]string) []string {
	defer perf.Track(nil, "env.MapToSlice")()

	return ConvertMapToSlice(m)
}

// loadEnvFilesFromDir loads .env files matching patterns from a single directory.
// Returns merged env vars and list of loaded files (sorted alphabetically for determinism).
func loadEnvFilesFromDir(dir string, patterns []string) (map[string]string, []string, error) {
	defer perf.Track(nil, "env.loadEnvFilesFromDir")()

	merged := make(map[string]string)
	var loadedFiles []string

	// Collect all matching files across all patterns.
	fileSet := make(map[string]struct{})
	for _, pattern := range patterns {
		fullPattern := filepath.Join(dir, pattern)
		matches, err := filepath.Glob(fullPattern)
		if err != nil {
			// Invalid pattern - skip it but don't fail.
			continue
		}
		for _, match := range matches {
			// Only include regular files, not directories.
			info, err := os.Stat(match)
			if err != nil || info.IsDir() {
				continue
			}
			fileSet[match] = struct{}{}
		}
	}

	// Sort files for deterministic load order.
	files := make([]string, 0, len(fileSet))
	for f := range fileSet {
		files = append(files, f)
	}
	sort.Strings(files)

	// Load each file.
	for _, file := range files {
		envMap, err := godotenv.Read(file)
		if err != nil {
			// File exists but couldn't be parsed - this is an error.
			return nil, nil, err
		}
		// Merge: later files override earlier.
		for k, v := range envMap {
			merged[k] = v
		}
		loadedFiles = append(loadedFiles, file)
	}

	return merged, loadedFiles, nil
}

// collectParentDirs collects directories from dir up to repoRoot (inclusive).
// Returns directories in order from repo root to dir (for correct merge precedence).
// Security: Never walks beyond repoRoot.
func collectParentDirs(dir, repoRoot string) ([]string, error) {
	defer perf.Track(nil, "env.collectParentDirs")()

	// Resolve to absolute paths for reliable comparison.
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	absRepoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, err
	}

	// Normalize paths (clean, no trailing slashes).
	absDir = filepath.Clean(absDir)
	absRepoRoot = filepath.Clean(absRepoRoot)

	// Collect directories from dir up to (but not past) repoRoot.
	var dirs []string
	current := absDir

	for {
		// Security check: ensure we're still within the repo root.
		if !isWithinOrEqual(current, absRepoRoot) {
			break
		}

		dirs = append(dirs, current)

		// Stop at repo root.
		if current == absRepoRoot {
			break
		}

		// Move to parent.
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root.
			break
		}
		current = parent
	}

	// Reverse to get repo root first (lowest priority).
	for i, j := 0, len(dirs)-1; i < j; i, j = i+1, j-1 {
		dirs[i], dirs[j] = dirs[j], dirs[i]
	}

	return dirs, nil
}

// isWithinOrEqual checks if path is within or equal to root.
func isWithinOrEqual(path, root string) bool {
	defer perf.Track(nil, "env.isWithinOrEqual")()

	// Ensure both paths are absolute and clean.
	path = filepath.Clean(path)
	root = filepath.Clean(root)

	// Check if path starts with root.
	if path == root {
		return true
	}

	// Ensure root ends with separator for prefix check.
	rootWithSep := root + string(filepath.Separator)
	return strings.HasPrefix(path, rootWithSep)
}
