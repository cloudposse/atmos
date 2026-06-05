package filesystem

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/bmatcuk/doublestar/v4"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// Default maximum number of entries in the glob LRU cache (defaultGlobCacheMaxEntries).
	// Override at startup with ATMOS_FS_GLOB_CACHE_MAX_ENTRIES.
	defaultGlobCacheMaxEntries = 1024

	// Default time-to-live for each cache entry (defaultGlobCacheTTL).
	// Override at startup with ATMOS_FS_GLOB_CACHE_TTL (e.g. "10m", "30s").
	defaultGlobCacheTTL = 5 * time.Minute

	// Minimum accepted TTL value (minGlobCacheTTL).  Values parsed from
	// ATMOS_FS_GLOB_CACHE_TTL that are positive but below this floor are clamped up.
	// A sub-second TTL would make the cache nearly useless and cause excessive I/O.
	minGlobCacheTTL = time.Second

	// Minimum accepted LRU capacity (minGlobCacheMaxEntries).  Values parsed
	// from ATMOS_FS_GLOB_CACHE_MAX_ENTRIES that are positive but below this floor
	// are clamped up to prevent near-empty caches that evict on nearly every call.
	minGlobCacheMaxEntries = 16
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
	// Bounded LRU cache for GetGlobMatches results (globMatchesLRU).
	// Replaces the unbounded sync.Map to prevent unbounded memory growth.
	// Access is mediated by a mutex so that the LRU's internal state is not
	// corrupted under concurrent use (hashicorp/golang-lru/v2 is thread-safe,
	// but we still need the mutex for atomic load+check+store sequences in our TTL logic).
	globMatchesLRU       *lru.Cache[string, globCacheEntry]
	globMatchesLRUMu     sync.RWMutex
	globMatchesEvictions int64 // Incremented atomically by the LRU eviction callback.
	globMatchesHits      int64 // Incremented atomically on each cache hit.
	globMatchesMisses    int64 // Incremented atomically on each cache miss.

	// Active TTL (globCacheTTL), configurable via ATMOS_FS_GLOB_CACHE_TTL.
	globCacheTTL = defaultGlobCacheTTL

	// Active LRU capacity (globCacheMaxEntries), configurable via ATMOS_FS_GLOB_CACHE_MAX_ENTRIES.
	globCacheMaxEntries = defaultGlobCacheMaxEntries

	// Controls whether empty-result sets are stored in the cache (globCacheEmptyEnabled).
	// Default true. Set ATMOS_FS_GLOB_CACHE_EMPTY=0 to disable.
	globCacheEmptyEnabled = true

	// PathMatchCache stores PathMatch results to avoid redundant pattern matching.
	// Cache key: pathMatchKey{pattern, name} -> match result (bool).
	// This cache is particularly effective during affected detection where the same
	// patterns are matched against the same files repeatedly in nested loops.
	pathMatchCache   = make(map[pathMatchKey]bool)
	pathMatchCacheMu sync.RWMutex
)

// resolveGlobCacheMaxEntries reads ATMOS_FS_GLOB_CACHE_MAX_ENTRIES and returns
// the resolved capacity, clamping up to minGlobCacheMaxEntries when a positive
// value below the floor is supplied.
func resolveGlobCacheMaxEntries() int {
	maxEntries := defaultGlobCacheMaxEntries
	//nolint:forbidigo // Direct env lookup required for cache configuration.
	v := os.Getenv("ATMOS_FS_GLOB_CACHE_MAX_ENTRIES")
	if v == "" {
		return maxEntries
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return maxEntries
	}
	if n < minGlobCacheMaxEntries {
		log.Warn("ATMOS_FS_GLOB_CACHE_MAX_ENTRIES below minimum, clamping up",
			"requested", n, "minimum", minGlobCacheMaxEntries)
		n = minGlobCacheMaxEntries
	}
	return n
}

// resolveGlobCacheTTL reads ATMOS_FS_GLOB_CACHE_TTL and returns the resolved
// duration, clamping up to minGlobCacheTTL when a positive value below the
// floor is supplied.
func resolveGlobCacheTTL() time.Duration {
	ttl := defaultGlobCacheTTL
	//nolint:forbidigo // Direct env lookup required for cache configuration.
	v := os.Getenv("ATMOS_FS_GLOB_CACHE_TTL")
	if v == "" {
		return ttl
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return ttl
	}
	if d < minGlobCacheTTL {
		log.Warn("ATMOS_FS_GLOB_CACHE_TTL below minimum, clamping up",
			"requested", d, "minimum", minGlobCacheTTL)
		d = minGlobCacheTTL
	}
	return d
}

// resolveGlobCacheEmptyEnabled reads ATMOS_FS_GLOB_CACHE_EMPTY and returns
// whether empty-result caching is enabled.  Only "1" or "true" explicitly
// enables; any other non-empty value disables.
func resolveGlobCacheEmptyEnabled() bool {
	//nolint:forbidigo // Direct env lookup required for cache configuration.
	v := os.Getenv("ATMOS_FS_GLOB_CACHE_EMPTY")
	if v == "" {
		return true
	}
	switch v {
	case "1", "true":
		return true
	default:
		return false
	}
}

// applyGlobCacheConfig reads ATMOS_FS_GLOB_CACHE_* environment variables and
// (re-)initializes the glob LRU cache accordingly.  It is called once from
// init() and may be called again from tests to pick up env changes.
func applyGlobCacheConfig() {
	maxEntries := resolveGlobCacheMaxEntries()
	ttl := resolveGlobCacheTTL()
	emptyEnabled := resolveGlobCacheEmptyEnabled()

	newLRU, err := lru.NewWithEvict[string, globCacheEntry](
		maxEntries,
		func(_ string, _ globCacheEntry) {
			atomic.AddInt64(&globMatchesEvictions, 1)
		},
	)

	globMatchesLRUMu.Lock()
	if err != nil {
		log.Error("glob LRU cache initialization failed; cache will be disabled", "error", err)
	} else {
		globMatchesLRU = newLRU
	}
	globCacheTTL = ttl
	globCacheMaxEntries = maxEntries
	globCacheEmptyEnabled = emptyEnabled
	globMatchesLRUMu.Unlock()
}

func init() {
	applyGlobCacheConfig()
}

// GetGlobMatches tries to read and return the Glob matches content from the cache if it exists,
// otherwise it finds and returns all files matching the pattern, stores the files in the cache and returns the files.
//
// Contract: when err == nil, the returned slice is non-nil; an empty result is returned as []string{}, not nil.
// On error (err != nil), the returned slice is nil.  Callers should check err first, then use len(result) == 0
// to detect no matches.
//
// Migration note: if your code uses "if result == nil" to detect no matches, update it to "if len(result) == 0".
// Callers should always use len(result) == 0 to detect no matches, not a nil comparison.
//
// Caching policy:
//   - Results are bounded to a configurable LRU (default 1024 entries, minimum 16, ATMOS_FS_GLOB_CACHE_MAX_ENTRIES).
//   - Each entry expires after a configurable TTL (default 5 minutes, minimum 1s, ATMOS_FS_GLOB_CACHE_TTL).
//   - Empty results are cached by default; set ATMOS_FS_GLOB_CACHE_EMPTY=0 to disable.
//   - Cached slices are cloned on read, so callers may safely mutate the returned slice.
func GetGlobMatches(pattern string) ([]string, error) {
	defer perf.Track(nil, "filesystem.GetGlobMatches")()

	// Normalize pattern for cache lookup to ensure consistent keys across platforms.
	normalizedPattern := filepath.ToSlash(pattern)

	// Try cache lookup (read lock on the LRU wrapper).
	globMatchesLRUMu.RLock()
	entry, found := globMatchesLRU.Get(normalizedPattern)
	ttl := globCacheTTL
	globMatchesLRUMu.RUnlock()

	if found && time.Now().Before(entry.expiry) {
		atomic.AddInt64(&globMatchesHits, 1)
		// Return a clone to prevent callers from mutating the cached slice.
		result := make([]string, len(entry.matches))
		copy(result, entry.matches)
		return result, nil
	}

	atomic.AddInt64(&globMatchesMisses, 1)

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

	// Only store in cache when: (a) there are matches, or (b) empty caching is enabled.
	globMatchesLRUMu.RLock()
	cacheEmpty := globCacheEmptyEnabled
	globMatchesLRUMu.RUnlock()

	if len(fullMatches) > 0 || cacheEmpty {
		// Store a copy of the slice in the cache (not the shared backing slice).
		// This prevents callers from mutating cached data and preserves empty results.
		cachedCopy := make([]string, len(fullMatches))
		copy(cachedCopy, fullMatches)

		globMatchesLRUMu.Lock()
		globMatchesLRU.Add(normalizedPattern, globCacheEntry{
			matches: cachedCopy,
			expiry:  time.Now().Add(ttl),
		})
		globMatchesLRUMu.Unlock()
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
