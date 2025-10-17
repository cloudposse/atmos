package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bmatcuk/doublestar/v4"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// pathMatchKey is a composite key for PathMatch cache to avoid collisions.
// Using a struct prevents issues when pattern or name contains delimiter characters.
type pathMatchKey struct {
	pattern string
	name    string
}

var (
	getGlobMatchesSyncMap = sync.Map{}

	// PathMatchCache stores PathMatch results to avoid redundant pattern matching.
	// Cache key: pathMatchKey{pattern, name} -> match result (bool).
	// This cache is particularly effective during affected detection where the same
	// patterns are matched against the same files repeatedly in nested loops.
	pathMatchCache   = make(map[pathMatchKey]bool)
	pathMatchCacheMu sync.RWMutex
)

// GetGlobMatches tries to read and return the Glob matches content from the sync map if it exists in the map,
// otherwise it finds and returns all files matching the pattern, stores the files in the map and returns the files.
func GetGlobMatches(pattern string) ([]string, error) {
	defer perf.Track(nil, "utils.GetGlobMatches")()

	existingMatches, found := getGlobMatchesSyncMap.Load(pattern)
	if found && existingMatches != nil {
		return strings.Split(existingMatches.(string), ","), nil
	}

	pattern = filepath.ToSlash(pattern)
	base, cleanPattern := doublestar.SplitPattern(pattern)
	f := os.DirFS(base)

	matches, err := doublestar.Glob(f, cleanPattern)
	if err != nil {
		return nil, err
	}

	if matches == nil {
		return nil, fmt.Errorf("%w: '%s' ('%s' + '%s')", errUtils.ErrFailedToFindImport, pattern, base, cleanPattern)
	}

	var fullMatches []string
	for _, match := range matches {
		fullMatches = append(fullMatches, filepath.Join(base, match))
	}

	getGlobMatchesSyncMap.Store(pattern, strings.Join(fullMatches, ","))

	return fullMatches, nil
}

// PathMatch returns true if `name` matches the file name `pattern`.
// PathMatch normalizes both pattern and name to use forward slashes for cross-platform
// compatibility. This ensures patterns work consistently on Windows, Linux, and macOS.
//
// Note: perf.Track() removed from this hot path to reduce overhead (called 2M+ times).
// Results are cached to avoid redundant pattern matching during affected detection.
func PathMatch(pattern, name string) (bool, error) {
	// Normalize both pattern and name to forward slashes for cross-platform compatibility.
	// Glob patterns universally use forward slashes, but Windows file paths use backslashes.
	// Converting both to forward slashes ensures patterns match correctly on all platforms.
	normalizedPattern := filepath.ToSlash(pattern)
	normalizedName := filepath.ToSlash(name)

	// Try cache first (read lock) - use normalized values for cache key.
	cacheKey := pathMatchKey{pattern: normalizedPattern, name: normalizedName}
	pathMatchCacheMu.RLock()
	result, found := pathMatchCache[cacheKey]
	pathMatchCacheMu.RUnlock()

	if found {
		return result, nil
	}

	// Cache miss - compute the result using normalized paths.
	match, err := doublestar.PathMatch(normalizedPattern, normalizedName)
	if err != nil {
		// Don't cache errors - they might be transient.
		return false, err
	}

	// Store result in cache (write lock).
	pathMatchCacheMu.Lock()
	pathMatchCache[cacheKey] = match
	pathMatchCacheMu.Unlock()

	return match, nil
}
