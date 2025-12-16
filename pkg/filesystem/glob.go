package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
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
	defer perf.Track(nil, "filesystem.GetGlobMatches")()

	// Normalize pattern for cache lookup to ensure consistent keys across platforms.
	normalizedPattern := filepath.ToSlash(pattern)

	existingMatches, found := getGlobMatchesSyncMap.Load(normalizedPattern)
	if found && existingMatches != nil {
		// Assert to []string and return a cloned copy so callers can't mutate cached data.
		cached, ok := existingMatches.([]string)
		if !ok {
			// If assertion fails, invalidate cache and fall through to recompute.
			getGlobMatchesSyncMap.Delete(normalizedPattern)
		}
		if ok {
			// Return a clone to prevent callers from mutating the cached slice.
			result := make([]string, len(cached))
			copy(result, cached)
			return result, nil
		}
	}

	base, cleanPattern := doublestar.SplitPattern(normalizedPattern)

	// Check if base directory exists before attempting glob.
	// os.DirFS will panic if the directory doesn't exist.
	if _, err := os.Stat(base); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: '%s' ('%s' + '%s')", errUtils.ErrFailedToFindImport, normalizedPattern, base, cleanPattern)
		}
		return nil, err
	}

	f := os.DirFS(base)

	matches, err := doublestar.Glob(f, cleanPattern)
	if err != nil {
		return nil, err
	}

	// doublestar.Glob returns nil on some platforms and []string{} on others when no matches.
	// Treat both as empty results - not an error.
	if matches == nil {
		matches = []string{}
	}

	var fullMatches []string
	for _, match := range matches {
		fullMatches = append(fullMatches, filepath.Join(base, match))
	}

	// Store a copy of the slice in the cache (not the shared backing slice).
	// This prevents callers from mutating cached data and preserves empty results.
	cachedCopy := make([]string, len(fullMatches))
	copy(cachedCopy, fullMatches)
	getGlobMatchesSyncMap.Store(normalizedPattern, cachedCopy)

	return fullMatches, nil
}

// PathMatch returns true if `name` matches the file name `pattern`.
// PathMatch normalizes both pattern and name to use forward slashes for cross-platform
// compatibility, then uses doublestar.Match (not PathMatch) which expects Unix-style paths.
// This ensures patterns work consistently on Windows, Linux, and macOS.
//
// Note: perf.Track() removed from this hot path to reduce overhead (called 2M+ times).
// Results are cached to avoid redundant pattern matching during affected detection.
func PathMatch(pattern, name string) (bool, error) {
	// Normalize both pattern and name to forward slashes for cross-platform compatibility.
	// Glob patterns universally use forward slashes, but Windows file paths use backslashes.
	// Converting both to forward slashes and using doublestar.Match (which expects Unix-style
	// paths) ensures patterns match correctly on all platforms.
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
	// Use doublestar.Match (not PathMatch) since we've already normalized to Unix-style paths.
	match, err := doublestar.Match(normalizedPattern, normalizedName)
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
