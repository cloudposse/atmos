package filesystem

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/bmatcuk/doublestar/v4"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// globCacheMaxEntries is the maximum number of entries in the glob matches LRU cache.
	// Once this limit is reached the least-recently-used entry is evicted automatically.
	globCacheMaxEntries = 1024

	// globCacheTTL is the time-to-live for each cache entry.
	// After this duration a stale entry is treated as a cache miss.
	globCacheTTL = 5 * time.Minute
)

// globCacheEntry holds a cached glob result together with its expiry timestamp.
type globCacheEntry struct {
	matches []string
	expiry  time.Time
}

// pathMatchKey is a composite key for PathMatch cache to avoid collisions.
// Using a struct prevents issues when pattern or name contains delimiter characters.
type pathMatchKey struct {
	pattern string
	name    string
}

var (
	// globMatchesLRU is a bounded LRU cache for GetGlobMatches results.
	// It replaces the unbounded sync.Map to prevent unbounded memory growth.
	// Access is mediated by a mutex so that the LRU's internal state is not
	// corrupted under concurrent use (hashicorp/golang-lru/v2 is thread-safe,
	// but we still need the mutex for atomic load+check+store sequences in our TTL logic).
	globMatchesLRU       *lru.Cache[string, globCacheEntry]
	globMatchesLRUMu     sync.RWMutex
	globMatchesLRUErr    error // non-nil only if lru.New fails (should never happen at runtime)
	globMatchesEvictions int64 // incremented atomically by the LRU eviction callback

	// PathMatchCache stores PathMatch results to avoid redundant pattern matching.
	// Cache key: pathMatchKey{pattern, name} -> match result (bool).
	// This cache is particularly effective during affected detection where the same
	// patterns are matched against the same files repeatedly in nested loops.
	pathMatchCache   = make(map[pathMatchKey]bool)
	pathMatchCacheMu sync.RWMutex
)

func init() {
	globMatchesLRU, globMatchesLRUErr = lru.NewWithEvict[string, globCacheEntry](
		globCacheMaxEntries,
		func(_ string, _ globCacheEntry) {
			atomic.AddInt64(&globMatchesEvictions, 1)
		},
	)
}

// GetGlobMatches tries to read and return the Glob matches content from the cache if it exists,
// otherwise it finds and returns all files matching the pattern, stores the files in the cache and returns the files.
//
// Contract: the returned slice is always non-nil (never nil). An empty result is returned as []string{}, not nil.
// This guarantee holds for both cache hits and misses, allowing callers to safely use len(result) without a nil check.
//
// Migration note: if your code uses "if result == nil" to detect no matches, update it to "if len(result) == 0".
// Callers should always use len(result) == 0 to detect no matches, not a nil comparison.
//
// Caching policy:
//   - All results (including empty matches) are cached.
//   - Cache is bounded to globCacheMaxEntries (1024) entries; LRU eviction applies.
//   - Each entry expires after globCacheTTL (5 minutes); a stale entry is treated as a cache miss.
//   - Cached slices are cloned on read, so callers may safely mutate the returned slice.
func GetGlobMatches(pattern string) ([]string, error) {
	defer perf.Track(nil, "filesystem.GetGlobMatches")()

	// Normalize pattern for cache lookup to ensure consistent keys across platforms.
	normalizedPattern := filepath.ToSlash(pattern)

	// Try cache lookup (read lock on the LRU wrapper).
	globMatchesLRUMu.RLock()
	entry, found := globMatchesLRU.Get(normalizedPattern)
	globMatchesLRUMu.RUnlock()

	if found && time.Now().Before(entry.expiry) {
		// Return a clone to prevent callers from mutating the cached slice.
		result := make([]string, len(entry.matches))
		copy(result, entry.matches)
		return result, nil
	}

	base, cleanPattern := doublestar.SplitPattern(normalizedPattern)

	// Check if base directory exists before attempting glob.
	// os.DirFS will panic if the directory doesn't exist.
	if _, err := os.Stat(base); err != nil {
		if os.IsNotExist(err) {
			return nil, errUtils.Build(errUtils.ErrFailedToFindImport).
				WithContext("paths", normalizedPattern).
				Err()
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

	fullMatches := make([]string, 0, len(matches))
	for _, match := range matches {
		fullMatches = append(fullMatches, filepath.Join(base, match))
	}

	// Store a copy of the slice in the cache (not the shared backing slice).
	// This prevents callers from mutating cached data and preserves empty results.
	cachedCopy := make([]string, len(fullMatches))
	copy(cachedCopy, fullMatches)

	globMatchesLRUMu.Lock()
	globMatchesLRU.Add(normalizedPattern, globCacheEntry{
		matches: cachedCopy,
		expiry:  time.Now().Add(globCacheTTL),
	})
	globMatchesLRUMu.Unlock()

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
