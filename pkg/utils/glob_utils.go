package utils

import (
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
//
// Note: unlike pkg/filesystem.GetGlobMatches, this function returns an error when no files match the pattern
// (consistent with its use as an import-path resolver). The returned slice may be nil when an error is returned.
// See pkg/filesystem.GetGlobMatches for the variant that returns ([]string{}, nil) instead of an error
// when no files match.
//
// Caching contract: only non-empty result sets are cached. The cache stores []string directly (not a
// comma-joined string) so that paths containing commas are preserved correctly on cache hits.
// Cached slices are cloned before being returned, so callers may safely mutate the returned slice without
// affecting the cache.
func GetGlobMatches(pattern string) ([]string, error) {
	defer perf.Track(nil, "utils.GetGlobMatches")()

	// Normalize pattern before cache lookup so that Windows backslash paths and
	// forward-slash paths resolve to the same cache key (mirrors pkg/filesystem behavior).
	pattern = filepath.ToSlash(pattern)

	existingMatches, found := getGlobMatchesSyncMap.Load(pattern)
	if found && existingMatches != nil {
		// Cache stores []string directly to avoid the comma-splitting bug:
		// paths containing commas would be split incorrectly if stored as a
		// comma-joined string and then re-split on read.
		if cached, ok := existingMatches.([]string); ok {
			// Return a clone so callers cannot mutate the cached slice.
			result := make([]string, len(cached))
			copy(result, cached)
			return result, nil
		}
		// Unexpected cache type: invalidate and recompute.
		getGlobMatchesSyncMap.Delete(pattern)
	}

	base, cleanPattern := doublestar.SplitPattern(pattern)
	f := os.DirFS(base)

	matches, err := doublestar.Glob(f, cleanPattern)
	if err != nil {
		return nil, err
	}

	if matches == nil {
		return nil, errUtils.Build(errUtils.ErrFailedToFindImport).
			WithHint("Verify that `base_path` and `stacks.base_path` in `atmos.yaml` are correct").
			WithHintf("If using `ATMOS_BASE_PATH`, ensure the path is correct relative to the working directory").
			WithContext("pattern", pattern).
			Err()
	}

	var fullMatches []string
	for _, match := range matches {
		fullMatches = append(fullMatches, filepath.Join(filepath.FromSlash(base), match))
	}

	// Only cache non-empty results. Empty results are not cached because
	// pkg/utils.GetGlobMatches treats "no matches" as an error, so there is
	// nothing useful to cache (the error is re-computed on every call).
	// Storing []string directly avoids the comma-splitting bug: paths that
	// contain commas would be mangled if stored/read as a joined string.
	if len(fullMatches) > 0 {
		// Store a clone so callers cannot mutate cached data.
		cached := make([]string, len(fullMatches))
		copy(cached, fullMatches)
		getGlobMatchesSyncMap.Store(pattern, cached)
	}

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
